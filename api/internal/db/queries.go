package db

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ListProducts returns the workspace catalog, ordered by name.
func (s *Store) ListProducts(ctx context.Context, workspaceID uuid.UUID) ([]Product, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id::text, name, price_minor, currency, stock
		FROM products
		WHERE workspace_id = $1
		ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	products := []Product{}
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Name, &p.PriceMinor, &p.Currency, &p.Stock); err != nil {
			return nil, err
		}
		p.Currency = strings.TrimSpace(p.Currency)
		products = append(products, p)
	}
	return products, rows.Err()
}

// ListOrderRequests returns one page and the total count for the same filter.
func (s *Store) ListOrderRequests(ctx context.Context, workspaceID uuid.UUID, f ListFilter) ([]OrderRequest, int, error) {
	status := statusArg(f.Status)

	var total int
	err := s.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM order_requests
		WHERE workspace_id = $1
		  AND ($2::text IS NULL OR status = $2)`, workspaceID, status).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT id, buyer_name, buyer_email, status, created_at
		FROM order_requests
		WHERE workspace_id = $1
		  AND ($2::text IS NULL OR status = $2)
		ORDER BY created_at DESC, id DESC
		LIMIT $3 OFFSET $4`, workspaceID, status, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := []OrderRequest{}
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		var item OrderRequest
		if err := rows.Scan(&id, &item.BuyerName, &item.BuyerEmail, &item.Status, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		item.ID = id.String()
		item.Lines = []Line{}
		items = append(items, item)
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if err := attachLines(ctx, s.Pool, ids, items); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// GetOrderRequest loads one request in a workspace.
func (s *Store) GetOrderRequest(ctx context.Context, workspaceID, requestID uuid.UUID) (OrderRequest, error) {
	items := []OrderRequest{{}}
	err := s.Pool.QueryRow(ctx, `
		SELECT id::text, buyer_name, buyer_email, status, created_at
		FROM order_requests
		WHERE id = $1 AND workspace_id = $2`, requestID, workspaceID).Scan(
		&items[0].ID, &items[0].BuyerName, &items[0].BuyerEmail, &items[0].Status, &items[0].CreatedAt)
	if err != nil {
		return OrderRequest{}, err
	}
	items[0].Lines = []Line{}
	if err := attachLines(ctx, s.Pool, []uuid.UUID{requestID}, items); err != nil {
		return OrderRequest{}, err
	}
	return items[0], nil
}

// LockRequestStatus locks the request row. pgx.ErrNoRows means it is not in this workspace.
func LockRequestStatus(ctx context.Context, q Querier, workspaceID, requestID uuid.UUID) (string, error) {
	var status string
	err := q.QueryRow(ctx, `
		SELECT status
		FROM order_requests
		WHERE id = $1 AND workspace_id = $2
		FOR UPDATE`, requestID, workspaceID).Scan(&status)
	return status, err
}

// ListRequestLines reads the snapshotted lines of one request.
func ListRequestLines(ctx context.Context, q Querier, requestID uuid.UUID) ([]Line, error) {
	grouped, err := linesByRequest(ctx, q, []uuid.UUID{requestID})
	if err != nil {
		return nil, err
	}
	lines := grouped[requestID]
	if lines == nil {
		return []Line{}, nil
	}
	return lines, nil
}

// LockProduct locks one product row. mode is UPDATE or SHARE.
// Callers lock product ids in sorted order so two transactions cannot deadlock.
func LockProduct(ctx context.Context, q Querier, workspaceID, productID uuid.UUID, mode string) (name string, priceMinor int64, currency string, stock int, err error) {
	if mode != "UPDATE" && mode != "SHARE" {
		return "", 0, "", 0, errBadLock
	}
	err = q.QueryRow(ctx, `
		SELECT name, price_minor, currency, stock
		FROM products
		WHERE id = $1 AND workspace_id = $2
		FOR `+mode, productID, workspaceID).Scan(&name, &priceMinor, &currency, &stock)
	if err != nil {
		return "", 0, "", 0, err
	}
	return name, priceMinor, strings.TrimSpace(currency), stock, nil
}

var errBadLock = errorsSentinel("bad lock mode")

// DecrementStock subtracts qty when the row still has enough. The product row must already be locked.
func DecrementStock(ctx context.Context, q Querier, workspaceID, productID uuid.UUID, qty int) error {
	tag, err := q.Exec(ctx, `
		UPDATE products
		SET stock = stock - $3
		WHERE id = $1 AND workspace_id = $2 AND stock >= $3`, productID, workspaceID, qty)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotUpdated
	}
	return nil
}

// InsertOrderRequest writes a new request and its lines. lines already carry the price snapshot.
func InsertOrderRequest(ctx context.Context, q Querier, workspaceID uuid.UUID, buyerName, buyerEmail string, lines []Line) (OrderRequest, error) {
	currency, total, err := applyTotals(lines)
	if err != nil {
		return OrderRequest{}, err
	}
	id := uuid.New()
	var createdAt time.Time
	err = q.QueryRow(ctx, `
		INSERT INTO order_requests (id, workspace_id, buyer_name, buyer_email, status)
		VALUES ($1, $2, $3, $4, 'new')
		RETURNING created_at`, id, workspaceID, buyerName, buyerEmail).Scan(&createdAt)
	if err != nil {
		return OrderRequest{}, err
	}
	if err := insertRequestLines(ctx, q, id, workspaceID, lines); err != nil {
		return OrderRequest{}, err
	}
	return OrderRequest{
		ID:         id.String(),
		BuyerName:  buyerName,
		BuyerEmail: buyerEmail,
		Status:     "new",
		CreatedAt:  createdAt,
		Currency:   currency,
		TotalMinor: total,
		Lines:      nonNilLines(lines),
	}, nil
}

// InsertDraftOrder copies lines onto a new draft. order_request_id is unique.
func InsertDraftOrder(ctx context.Context, q Querier, workspaceID, requestID uuid.UUID, lines []Line) (DraftOrder, error) {
	currency, total, err := applyTotals(lines)
	if err != nil {
		return DraftOrder{}, err
	}
	id := uuid.New()
	var createdAt time.Time
	err = q.QueryRow(ctx, `
		INSERT INTO orders (id, workspace_id, order_request_id, status)
		VALUES ($1, $2, $3, 'draft')
		RETURNING created_at`, id, workspaceID, requestID).Scan(&createdAt)
	if err != nil {
		return DraftOrder{}, err
	}
	for _, line := range lines {
		productID, err := uuid.Parse(line.ProductID)
		if err != nil {
			return DraftOrder{}, err
		}
		_, err = q.Exec(ctx, `
			INSERT INTO order_lines
				(id, order_id, workspace_id, product_id, quantity, unit_price_minor, currency)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			uuid.New(), id, workspaceID, productID, line.Quantity, line.UnitPriceMinor, line.Currency)
		if err != nil {
			return DraftOrder{}, err
		}
	}
	return DraftOrder{
		ID:             id.String(),
		OrderRequestID: requestID.String(),
		Status:         "draft",
		CreatedAt:      createdAt,
		Currency:       currency,
		TotalMinor:     total,
		Lines:          nonNilLines(lines),
	}, nil
}

// MarkAccepted moves a new request to accepted. The row must already be locked.
func MarkAccepted(ctx context.Context, q Querier, workspaceID, requestID uuid.UUID) error {
	tag, err := q.Exec(ctx, `
		UPDATE order_requests
		SET status = 'accepted'
		WHERE id = $1 AND workspace_id = $2 AND status = 'new'`, requestID, workspaceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotUpdated
	}
	return nil
}

// RejectIfNew rejects a request that is still new. It returns false when no row changed.
func RejectIfNew(ctx context.Context, q Querier, workspaceID, requestID uuid.UUID) (bool, error) {
	tag, err := q.Exec(ctx, `
		UPDATE order_requests
		SET status = 'rejected'
		WHERE id = $1 AND workspace_id = $2 AND status = 'new'`, requestID, workspaceID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// RequestExists reports whether the id belongs to the workspace, whatever its status.
func RequestExists(ctx context.Context, q Querier, workspaceID, requestID uuid.UUID) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM order_requests WHERE id = $1 AND workspace_id = $2
		)`, requestID, workspaceID).Scan(&exists)
	return exists, err
}

func insertRequestLines(ctx context.Context, q Querier, requestID, workspaceID uuid.UUID, lines []Line) error {
	for _, line := range lines {
		productID, err := uuid.Parse(line.ProductID)
		if err != nil {
			return err
		}
		_, err = q.Exec(ctx, `
			INSERT INTO order_request_lines
				(id, order_request_id, workspace_id, product_id, quantity, unit_price_minor, currency)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			uuid.New(), requestID, workspaceID, productID, line.Quantity, line.UnitPriceMinor, line.Currency)
		if err != nil {
			return err
		}
	}
	return nil
}

func attachLines(ctx context.Context, q Querier, ids []uuid.UUID, items []OrderRequest) error {
	grouped, err := linesByRequest(ctx, q, ids)
	if err != nil {
		return err
	}
	for i := range items {
		id, err := uuid.Parse(items[i].ID)
		if err != nil {
			return err
		}
		lines := grouped[id]
		if lines == nil {
			lines = []Line{}
		}
		currency, total, err := applyTotals(lines)
		if err != nil {
			return err
		}
		items[i].Lines = lines
		items[i].Currency = currency
		items[i].TotalMinor = total
	}
	return nil
}

func linesByRequest(ctx context.Context, q Querier, ids []uuid.UUID) (map[uuid.UUID][]Line, error) {
	grouped := map[uuid.UUID][]Line{}
	if len(ids) == 0 {
		return grouped, nil
	}
	rows, err := q.Query(ctx, `
		SELECT l.order_request_id, l.product_id::text, p.name, l.quantity, l.unit_price_minor, l.currency
		FROM order_request_lines l
		JOIN products p ON p.id = l.product_id AND p.workspace_id = l.workspace_id
		WHERE l.order_request_id = ANY($1::uuid[])
		ORDER BY p.name`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var requestID uuid.UUID
		var line Line
		if err := rows.Scan(&requestID, &line.ProductID, &line.ProductName, &line.Quantity, &line.UnitPriceMinor, &line.Currency); err != nil {
			return nil, err
		}
		line.Currency = strings.TrimSpace(line.Currency)
		grouped[requestID] = append(grouped[requestID], line)
	}
	return grouped, rows.Err()
}

func statusArg(status *string) any {
	if status == nil {
		return nil
	}
	return *status
}

func nonNilLines(lines []Line) []Line {
	if lines == nil {
		return []Line{}
	}
	return lines
}
