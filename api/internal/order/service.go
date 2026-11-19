package order

import (
	"bytes"
	"context"
	"errors"
	"net/mail"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/ploiid10/beautyscale-order-requests/api/internal/db"
)

const (
	maxNameLen = 200
	maxEmail   = 254
	maxLines   = 50
	maxQty     = 10000
)

// Service is the order-request use case.
type Service struct {
	store *db.Store
}

// NewService wires the store.
func NewService(store *db.Store) *Service {
	return &Service{store: store}
}

// CreateInput is a buyer submission.
type CreateInput struct {
	BuyerName  string      `json:"buyerName"`
	BuyerEmail string      `json:"buyerEmail"`
	Lines      []LineInput `json:"lines"`
}

// LineInput is one requested product.
type LineInput struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
}

// Products lists the workspace catalog.
func (s *Service) Products(ctx context.Context, workspaceID uuid.UUID) ([]db.Product, error) {
	items, err := s.store.ListProducts(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []db.Product{}
	}
	return items, nil
}

// List returns a page of requests. status, limit, and offset are raw query values.
func (s *Service) List(ctx context.Context, workspaceID uuid.UUID, statusRaw, limitRaw, offsetRaw string) ([]db.OrderRequest, int, int, int, error) {
	limit, offset, err := parsePage(limitRaw, offsetRaw)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	var status *string
	if statusRaw != "" {
		switch statusRaw {
		case "new", "accepted", "rejected":
			status = &statusRaw
		default:
			return nil, 0, 0, 0, Invalid("status must be new, accepted, or rejected")
		}
	}
	items, total, err := s.store.ListOrderRequests(ctx, workspaceID, db.ListFilter{
		Status: status,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, 0, 0, 0, err
	}
	if items == nil {
		items = []db.OrderRequest{}
	}
	return items, limit, offset, total, nil
}

// Get loads one request. Another workspace's id is not found.
func (s *Service) Get(ctx context.Context, workspaceID uuid.UUID, requestID string) (db.OrderRequest, error) {
	id, err := parseID(requestID)
	if err != nil {
		return db.OrderRequest{}, err
	}
	item, err := s.store.GetOrderRequest(ctx, workspaceID, id)
	if err != nil {
		if errorsIsNoRows(err) {
			return db.OrderRequest{}, NotFound("order request not found")
		}
		return db.OrderRequest{}, err
	}
	return item, nil
}

// Create checks the buyer payload, snapshots prices, and stores a new request.
func (s *Service) Create(ctx context.Context, workspaceID uuid.UUID, in CreateInput) (db.OrderRequest, error) {
	name := strings.TrimSpace(in.BuyerName)
	if name == "" || len(name) > maxNameLen {
		return db.OrderRequest{}, Invalid("buyer name is required")
	}
	email, ok := normalizeEmail(in.BuyerEmail)
	if !ok {
		return db.OrderRequest{}, Invalid("buyer email is invalid")
	}
	ids, qty, err := normalizeLines(in.Lines)
	if err != nil {
		return db.OrderRequest{}, err
	}

	tx, err := s.store.Begin(ctx)
	if err != nil {
		return db.OrderRequest{}, err
	}
	defer tx.Rollback(ctx)

	lines := make([]db.Line, 0, len(ids))
	var currency string
	for _, id := range ids {
		productName, price, productCurrency, _, err := db.LockProduct(ctx, tx, workspaceID, id, "SHARE")
		if err != nil {
			if errorsIsNoRows(err) {
				return db.OrderRequest{}, Invalid("unknown product")
			}
			return db.OrderRequest{}, err
		}
		if currency == "" {
			currency = productCurrency
		} else if currency != productCurrency {
			return db.OrderRequest{}, Invalid("a request can only use one currency")
		}
		lines = append(lines, db.Line{
			ProductID:      id.String(),
			ProductName:    productName,
			Quantity:       qty[id],
			UnitPriceMinor: price,
			Currency:       productCurrency,
		})
	}

	created, err := db.InsertOrderRequest(ctx, tx, workspaceID, name, email, lines)
	if err != nil {
		if isConstraintViolation(err, "order_requests_open_same_idx") {
			return db.OrderRequest{}, Conflict("This request was already submitted")
		}
		return db.OrderRequest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.OrderRequest{}, err
	}
	return created, nil
}

func normalizeLines(in []LineInput) ([]uuid.UUID, map[uuid.UUID]int, error) {
	if len(in) == 0 {
		return nil, nil, Invalid("at least one line is required")
	}
	if len(in) > maxLines {
		return nil, nil, Invalid("too many lines")
	}
	ids := make([]uuid.UUID, 0, len(in))
	qty := make(map[uuid.UUID]int, len(in))
	seen := make(map[uuid.UUID]struct{}, len(in))
	for _, line := range in {
		id, err := uuid.Parse(strings.TrimSpace(line.ProductID))
		if err != nil || id == uuid.Nil {
			return nil, nil, Invalid("product id must be a UUID")
		}
		if line.Quantity < 1 || line.Quantity > maxQty {
			return nil, nil, Invalid("quantity must be between 1 and 10000")
		}
		if _, ok := seen[id]; ok {
			return nil, nil, Invalid("duplicate product")
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
		qty[id] = line.Quantity
	}
	sortUUIDs(ids)
	return ids, qty, nil
}

func normalizeEmail(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxEmail || strings.ContainsAny(raw, " \t\r\n") {
		return "", false
	}
	addr, err := mail.ParseAddress(raw)
	if err != nil || addr.Address != raw {
		return "", false
	}
	at := strings.LastIndex(addr.Address, "@")
	if at <= 0 || at >= len(addr.Address)-1 {
		return "", false
	}
	domain := addr.Address[at+1:]
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || !strings.Contains(domain, ".") {
		return "", false
	}
	return addr.Address, true
}

func parsePage(limitRaw, offsetRaw string) (int, int, error) {
	limit := 20
	offset := 0
	if limitRaw != "" {
		n, err := strconv.Atoi(limitRaw)
		if err != nil || n < 1 || n > 100 {
			return 0, 0, Invalid("limit must be between 1 and 100")
		}
		limit = n
	}
	if offsetRaw != "" {
		n, err := strconv.Atoi(offsetRaw)
		if err != nil || n < 0 {
			return 0, 0, Invalid("offset must be zero or greater")
		}
		offset = n
	}
	return limit, offset, nil
}

func parseID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, Invalid("id must be a UUID")
	}
	return id, nil
}

func sortUUIDs(ids []uuid.UUID) {
	sort.Slice(ids, func(i, j int) bool {
		return bytes.Compare(ids[i][:], ids[j][:]) < 0
	})
}

func errorsIsNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
