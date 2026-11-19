package order

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/ploiid10/beautyscale-order-requests/api/internal/db"
)

// Accept reserves stock and creates one draft order.
// The request row is locked first. Product rows are then locked one at a time in id order,
// which keeps two accepts from deadlocking. The unique order_request_id is the backstop
// if a second insert races past the status check.
func (s *Service) Accept(ctx context.Context, workspaceID uuid.UUID, requestID string) (db.DraftOrder, error) {
	id, err := parseID(requestID)
	if err != nil {
		return db.DraftOrder{}, err
	}

	tx, err := s.store.Begin(ctx)
	if err != nil {
		return db.DraftOrder{}, err
	}
	defer tx.Rollback(ctx)

	status, err := db.LockRequestStatus(ctx, tx, workspaceID, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.DraftOrder{}, NotFound("order request not found")
		}
		return db.DraftOrder{}, err
	}
	if status != "new" {
		return db.DraftOrder{}, Conflict("order request is no longer new")
	}

	lines, err := db.ListRequestLines(ctx, tx, id)
	if err != nil {
		return db.DraftOrder{}, err
	}
	if len(lines) == 0 {
		return db.DraftOrder{}, Conflict("order request has no lines")
	}

	need := map[uuid.UUID]int{}
	productIDs := []uuid.UUID{}
	for _, line := range lines {
		productID, err := uuid.Parse(line.ProductID)
		if err != nil {
			return db.DraftOrder{}, err
		}
		if _, ok := need[productID]; !ok {
			productIDs = append(productIDs, productID)
		}
		need[productID] += line.Quantity
	}
	sortUUIDs(productIDs)

	names := make(map[uuid.UUID]string, len(productIDs))
	stock := make(map[uuid.UUID]int, len(productIDs))
	for _, productID := range productIDs {
		name, _, _, qty, err := db.LockProduct(ctx, tx, workspaceID, productID, "UPDATE")
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return db.DraftOrder{}, Conflict("product is no longer available")
			}
			return db.DraftOrder{}, err
		}
		names[productID] = name
		stock[productID] = qty
	}
	for _, productID := range productIDs {
		if need[productID] > stock[productID] {
			return db.DraftOrder{}, Conflict(fmtStock(names[productID]))
		}
	}
	for _, productID := range productIDs {
		if err := db.DecrementStock(ctx, tx, workspaceID, productID, need[productID]); err != nil {
			if errors.Is(err, db.ErrNotUpdated) {
				return db.DraftOrder{}, Conflict(fmtStock(names[productID]))
			}
			return db.DraftOrder{}, err
		}
	}

	draft, err := db.InsertDraftOrder(ctx, tx, workspaceID, id, lines)
	if err != nil {
		if isUniqueViolation(err) {
			return db.DraftOrder{}, Conflict("order request is no longer new")
		}
		return db.DraftOrder{}, err
	}
	if err := db.MarkAccepted(ctx, tx, workspaceID, id); err != nil {
		if errors.Is(err, db.ErrNotUpdated) {
			return db.DraftOrder{}, Conflict("order request is no longer new")
		}
		return db.DraftOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.DraftOrder{}, err
	}
	return draft, nil
}

// Reject closes a new request and leaves stock unchanged.
func (s *Service) Reject(ctx context.Context, workspaceID uuid.UUID, requestID string) (Decision, error) {
	id, err := parseID(requestID)
	if err != nil {
		return Decision{}, err
	}

	tx, err := s.store.Begin(ctx)
	if err != nil {
		return Decision{}, err
	}
	defer tx.Rollback(ctx)

	// Lock first so a concurrent accept cannot commit between the update and the existence check.
	if _, err := db.LockRequestStatus(ctx, tx, workspaceID, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Decision{}, NotFound("order request not found")
		}
		return Decision{}, err
	}

	updated, err := db.RejectIfNew(ctx, tx, workspaceID, id)
	if err != nil {
		return Decision{}, err
	}
	if !updated {
		return Decision{}, Conflict("order request is no longer new")
	}
	if err := tx.Commit(ctx); err != nil {
		return Decision{}, err
	}
	return Decision{ID: id.String(), Status: "rejected"}, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
