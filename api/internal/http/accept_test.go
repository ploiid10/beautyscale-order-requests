package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ploiid10/beautyscale-order-requests/api/internal/db"
	httpapi "github.com/ploiid10/beautyscale-order-requests/api/internal/http"
	"github.com/ploiid10/beautyscale-order-requests/api/internal/order"
)

const (
	adminURL = "postgres://orders:orders@localhost:5432/postgres?sslmode=disable"
	testURL  = "postgres://orders:orders@localhost:5432/orders_test?sslmode=disable"

	ws1 = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1"
	ws2 = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb2"
	p1  = "cccccccc-cccc-4ccc-8ccc-ccccccccccc1"
	p2  = "dddddddd-dddd-4ddd-8ddd-ddddddddddd2"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()
	if err := setup(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	testPool.Close()
	os.Exit(code)
}

func setup(ctx context.Context) error {
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return fmt.Errorf("connect postgres (start it with `docker compose up -d postgres`): %w", err)
	}
	defer admin.Close(ctx)

	_, err = admin.Exec(ctx, `CREATE DATABASE orders_test`)
	if err != nil {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42P04" {
			return fmt.Errorf("create orders_test: %w", err)
		}
	}
	if err := db.Migrate(testURL, migrationsDir()); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, testURL)
	if err != nil {
		return err
	}
	testPool = pool
	return nil
}

func TestAcceptReservesStock(t *testing.T) {
	reset(t)
	insertProduct(t, ws1, p1, "Rose Serum", 500, 5)
	srv := newServer(t)

	id := createRequest(t, srv, ws1, []line{{p1, 2}})
	status, body := doJSON(t, http.MethodPost, srv.URL+"/api/order-requests/"+id+"/accept", ws1, nil)
	if status != http.StatusOK {
		t.Fatalf("accept status %d body %s", status, body)
	}
	var draft struct {
		Status         string `json:"status"`
		OrderRequestID string `json:"orderRequestId"`
		TotalMinor     int64  `json:"totalMinor"`
	}
	mustDecode(t, body, &draft)
	if draft.Status != "draft" || draft.OrderRequestID != id || draft.TotalMinor != 1000 {
		t.Fatalf("draft = %+v", draft)
	}
	if got := stockOf(t, p1); got != 3 {
		t.Fatalf("stock = %d, want 3", got)
	}
	if got := requestStatus(t, id); got != "accepted" {
		t.Fatalf("request status = %s", got)
	}
	if got := orderCount(t, id); got != 1 {
		t.Fatalf("orders = %d", got)
	}
}

func TestAcceptTwiceIsBlocked(t *testing.T) {
	reset(t)
	insertProduct(t, ws1, p1, "Rose Serum", 500, 5)
	srv := newServer(t)
	id := createRequest(t, srv, ws1, []line{{p1, 2}})

	status, _ := doJSON(t, http.MethodPost, srv.URL+"/api/order-requests/"+id+"/accept", ws1, nil)
	if status != http.StatusOK {
		t.Fatalf("first accept status %d", status)
	}
	status, body := doJSON(t, http.MethodPost, srv.URL+"/api/order-requests/"+id+"/accept", ws1, nil)
	if status != http.StatusConflict {
		t.Fatalf("second accept status %d body %s", status, body)
	}
	if stockOf(t, p1) != 3 {
		t.Fatalf("stock changed on second accept: %d", stockOf(t, p1))
	}
	if orderCount(t, id) != 1 {
		t.Fatalf("orders = %d, want 1", orderCount(t, id))
	}
}

func TestSameOpenRequestCannotBeSubmittedTwice(t *testing.T) {
	reset(t)
	insertProduct(t, ws1, p1, "Rose Serum", 500, 5)
	insertProduct(t, ws1, p2, "Silk Cleanser", 300, 5)
	srv := newServer(t)

	first := map[string]any{
		"buyerName":  "Ada Buyer",
		"buyerEmail": "Ada@Example.com",
		"lines": []map[string]any{
			{"productId": p2, "quantity": 1},
			{"productId": p1, "quantity": 2},
		},
	}
	status, body := doJSON(t, http.MethodPost, srv.URL+"/api/order-requests", ws1, first)
	if status != http.StatusCreated {
		t.Fatalf("create status %d body %s", status, body)
	}
	var created struct {
		ID string `json:"id"`
	}
	mustDecode(t, body, &created)

	second := map[string]any{
		"buyerName":  "Ada B.",
		"buyerEmail": "ada@example.com",
		"lines": []map[string]any{
			{"productId": p1, "quantity": 2},
			{"productId": p2, "quantity": 1},
		},
	}
	status, body = doJSON(t, http.MethodPost, srv.URL+"/api/order-requests", ws1, second)
	if status != http.StatusConflict {
		t.Fatalf("duplicate status %d body %s", status, body)
	}
	if !bytes.Contains(body, []byte("This request was already submitted")) {
		t.Fatalf("duplicate body %s", body)
	}

	otherBuyer := map[string]any{
		"buyerName":  "Bea Shop",
		"buyerEmail": "bea@example.com",
		"lines":      second["lines"],
	}
	status, body = doJSON(t, http.MethodPost, srv.URL+"/api/order-requests", ws1, otherBuyer)
	if status != http.StatusCreated {
		t.Fatalf("other buyer status %d body %s", status, body)
	}

	changedQty := map[string]any{
		"buyerName":  "Ada Buyer",
		"buyerEmail": "ada@example.com",
		"lines": []map[string]any{
			{"productId": p1, "quantity": 3},
			{"productId": p2, "quantity": 1},
		},
	}
	status, body = doJSON(t, http.MethodPost, srv.URL+"/api/order-requests", ws1, changedQty)
	if status != http.StatusCreated {
		t.Fatalf("changed qty status %d body %s", status, body)
	}

	status, body = doJSON(t, http.MethodPost, srv.URL+"/api/order-requests/"+created.ID+"/reject", ws1, nil)
	if status != http.StatusOK {
		t.Fatalf("reject status %d body %s", status, body)
	}
	status, body = doJSON(t, http.MethodPost, srv.URL+"/api/order-requests", ws1, second)
	if status != http.StatusCreated {
		t.Fatalf("resubmit after reject status %d body %s", status, body)
	}
}

func TestAcceptInsufficientStockRollsBack(t *testing.T) {
	reset(t)
	insertProduct(t, ws1, p1, "Rose Serum", 500, 5)
	insertProduct(t, ws1, p2, "Night Oil", 200, 1)
	srv := newServer(t)
	// The first line fits. The second does not. Neither stock may change.
	id := createRequest(t, srv, ws1, []line{{p1, 2}, {p2, 3}})

	status, body := doJSON(t, http.MethodPost, srv.URL+"/api/order-requests/"+id+"/accept", ws1, nil)
	if status != http.StatusConflict {
		t.Fatalf("status %d body %s", status, body)
	}
	var payload struct {
		Error string `json:"error"`
	}
	mustDecode(t, body, &payload)
	if payload.Error == "" {
		t.Fatal("missing error message")
	}
	if stockOf(t, p1) != 5 || stockOf(t, p2) != 1 {
		t.Fatalf("stock changed: p1=%d p2=%d", stockOf(t, p1), stockOf(t, p2))
	}
	if requestStatus(t, id) != "new" {
		t.Fatalf("status = %s, want new", requestStatus(t, id))
	}
	if orderCount(t, id) != 0 {
		t.Fatalf("orders = %d, want 0", orderCount(t, id))
	}
}

func TestWorkspaceIsolation(t *testing.T) {
	reset(t)
	insertProduct(t, ws1, p1, "Rose Serum", 500, 5)
	insertProduct(t, ws2, p2, "Glacier Balm", 2500, 30)
	srv := newServer(t)
	id := createRequest(t, srv, ws1, []line{{p1, 1}})

	status, body := doJSON(t, http.MethodGet, srv.URL+"/api/order-requests/"+id, ws2, nil)
	if status != http.StatusNotFound {
		t.Fatalf("read status %d body %s", status, body)
	}

	status, body = doJSON(t, http.MethodGet, srv.URL+"/api/order-requests", ws2, nil)
	if status != http.StatusOK {
		t.Fatalf("list status %d body %s", status, body)
	}
	var list struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		Total int `json:"total"`
	}
	mustDecode(t, body, &list)
	if list.Total != 0 || len(list.Items) != 0 {
		t.Fatalf("other workspace saw requests: %+v", list)
	}

	status, body = doJSON(t, http.MethodPost, srv.URL+"/api/order-requests/"+id+"/accept", ws2, nil)
	if status != http.StatusNotFound {
		t.Fatalf("accept status %d body %s", status, body)
	}
	if stockOf(t, p1) != 5 {
		t.Fatalf("stock changed from the other workspace: %d", stockOf(t, p1))
	}
	if requestStatus(t, id) != "new" {
		t.Fatalf("status = %s", requestStatus(t, id))
	}

	status, _ = doJSON(t, http.MethodGet, srv.URL+"/api/order-requests/"+id, ws1, nil)
	if status != http.StatusOK {
		t.Fatalf("owner read status %d", status)
	}
}

func TestConcurrentAcceptReservesStockOnce(t *testing.T) {
	reset(t)
	insertProduct(t, ws1, p1, "Rose Serum", 500, 10)
	srv := newServer(t)
	id := createRequest(t, srv, ws1, []line{{p1, 4}})

	var wg sync.WaitGroup
	codes := make([]int, 2)
	errs := make([]error, 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			defer wg.Done()
			codes[i], errs[i] = postStatus(http.MethodPost, srv.URL+"/api/order-requests/"+id+"/accept", ws1)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	ok, conflict := 0, 0
	for _, code := range codes {
		switch code {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			conflict++
		default:
			t.Fatalf("unexpected statuses %v", codes)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("statuses = %v, want one 200 and one 409", codes)
	}
	if stockOf(t, p1) != 6 {
		t.Fatalf("stock = %d, want 6", stockOf(t, p1))
	}
	if orderCount(t, id) != 1 {
		t.Fatalf("orders = %d, want 1", orderCount(t, id))
	}
}

func TestRejectLeavesStock(t *testing.T) {
	reset(t)
	insertProduct(t, ws1, p1, "Rose Serum", 500, 5)
	srv := newServer(t)
	id := createRequest(t, srv, ws1, []line{{p1, 2}})

	status, _ := doJSON(t, http.MethodPost, srv.URL+"/api/order-requests/"+id+"/reject", ws1, nil)
	if status != http.StatusOK {
		t.Fatalf("reject status %d", status)
	}
	if stockOf(t, p1) != 5 {
		t.Fatalf("stock = %d, want 5", stockOf(t, p1))
	}
	if requestStatus(t, id) != "rejected" {
		t.Fatalf("status = %s", requestStatus(t, id))
	}
	status, _ = doJSON(t, http.MethodPost, srv.URL+"/api/order-requests/"+id+"/accept", ws1, nil)
	if status != http.StatusConflict {
		t.Fatalf("accept after reject status %d", status)
	}
}

type line struct {
	id  string
	qty int
}

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(httpapi.NewHandler(order.NewService(db.NewStore(testPool))))
	t.Cleanup(srv.Close)
	return srv
}

func createRequest(t *testing.T, srv *httptest.Server, workspace string, lines []line) string {
	t.Helper()
	payload := map[string]any{
		"buyerName":  "Ada Buyer",
		"buyerEmail": "ada@example.com",
	}
	rawLines := make([]map[string]any, 0, len(lines))
	for _, ln := range lines {
		rawLines = append(rawLines, map[string]any{"productId": ln.id, "quantity": ln.qty})
	}
	payload["lines"] = rawLines
	status, body := doJSON(t, http.MethodPost, srv.URL+"/api/order-requests", workspace, payload)
	if status != http.StatusCreated {
		t.Fatalf("create status %d body %s", status, body)
	}
	var created struct {
		ID string `json:"id"`
	}
	mustDecode(t, body, &created)
	if created.ID == "" {
		t.Fatal("missing id")
	}
	return created.ID
}

func postStatus(method, url, workspace string) (int, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("X-Workspace-ID", workspace)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	_, err = io.ReadAll(res.Body)
	return res.StatusCode, err
}

func doJSON(t *testing.T, method, url, workspace string, body any) (int, []byte) {
	t.Helper()
	var buf io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		buf = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Workspace-ID", workspace)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, data
}

func mustDecode(t *testing.T, data []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(data, dst); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
}

func reset(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
		TRUNCATE TABLE
			order_lines,
			orders,
			order_request_lines,
			order_requests,
			products,
			workspaces
		RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatal(err)
	}
}

func insertProduct(t *testing.T, workspace, product, name string, price int64, stock int) {
	t.Helper()
	ctx := context.Background()
	_, err := testPool.Exec(ctx, `
		INSERT INTO workspaces (id, name) VALUES ($1, $2)
		ON CONFLICT (id) DO NOTHING`, workspace, "Test brand")
	if err != nil {
		t.Fatal(err)
	}
	_, err = testPool.Exec(ctx, `
		INSERT INTO products (id, workspace_id, name, price_minor, currency, stock)
		VALUES ($1, $2, $3, $4, 'USD', $5)`, product, workspace, name, price, stock)
	if err != nil {
		t.Fatal(err)
	}
}

func stockOf(t *testing.T, product string) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), `SELECT stock FROM products WHERE id = $1`, product).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func requestStatus(t *testing.T, id string) string {
	t.Helper()
	var status string
	if err := testPool.QueryRow(context.Background(), `SELECT status FROM order_requests WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func orderCount(t *testing.T, requestID string) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), `SELECT COUNT(*) FROM orders WHERE order_request_id = $1`, requestID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func migrationsDir() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "migrations")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("go.mod not found")
		}
		dir = parent
	}
}
