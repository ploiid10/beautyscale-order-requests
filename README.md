# Brand order requests

A buyer asks for products inside a brand workspace. Sales accepts the request, which creates a draft order and reserves stock, or rejects it. One workspace never sees another's data. There is no login. Every API call sends `X-Workspace-ID`.

## Prerequisites

- Docker with Compose v2
- Go 1.27+ and Node.js 22+, only if you run the API and web outside Docker

## Run

From this directory:

```bash
docker compose up --build
```

- Web: http://localhost:5173
- API: http://localhost:8080
- Postgres: `localhost:5432`, user `orders`, password `orders`, database `orders`

The defaults are in `docker-compose.yml`. Copy `.env.example` to `.env` only to change ports or credentials. Stop with Ctrl-C, or `docker compose down`. The database volume is kept. `docker compose down -v` deletes it.

On startup the API applies SQL migrations and an idempotent seed. The seed will not reset stock after an accept.

## Local development

```bash
docker compose up -d postgres
```

API, from `api/`:

```bash
go run ./cmd/server
```

Web, from `web/`:

```bash
npm install
npm run dev
```

Vite proxies `/api` to `http://localhost:8080`.

## Tests

```bash
docker compose up -d postgres
cd api && go test ./...
```

Tests use a separate database, `orders_test`, on that Postgres. They cover stock reservation, a second accept, an accept that rolls back when stock is short, concurrent accepts, reject leaving stock alone, and workspace isolation.

## Seeded workspaces

The workspace dropdown in the header is for the demo. There is no login, so the select is how you act as one brand or the other and check that they cannot read or write each other's requests. It only chooses the `X-Workspace-ID` sent with every API call. Isolation is enforced by the API: each query is limited to that workspace, and another workspace's request is 404.

- Lumen Atelier: `8f1c0b2e-6a4d-4c1e-9b7a-1d2e3f4a5b6c` (USD)
- Northline Beauty: `3c9e1a70-2b54-4f8d-a6e1-7b8c9d0e1f2a` (EUR)

```bash
curl -s \
  -H "X-Workspace-ID: 8f1c0b2e-6a4d-4c1e-9b7a-1d2e3f4a5b6c" \
  http://localhost:8080/api/products
```

## API

Missing or invalid `X-Workspace-ID` is 400. A row that belongs to another workspace is 404.

- `GET /api/products`
- `POST /api/order-requests` with `buyerName`, `buyerEmail`, and `lines` of `productId` and `quantity`
- `GET /api/order-requests?status=new&limit=20&offset=0` (`status` is optional)
- `GET /api/order-requests/{id}`
- `POST /api/order-requests/{id}/accept`
- `POST /api/order-requests/{id}/reject`

Accept returns 409 when the request is no longer new or stock is too low. The stock error names the product. Reject returns 409 once the request has already been decided, and it does not change stock. Create returns 409 when that buyer already has a `new` request with the same products and quantities.

## Design choices

- Money is a `BIGINT` of minor units plus a 3-letter currency. The UI formats it with `Intl.NumberFormat` and treats the exponent as 2, which matches the USD and EUR seed.
- Accept and reject each run in one transaction. Accept locks the request row, then locks product rows one at a time in id order, checks stock, decrements it, inserts the draft order, and marks the request accepted. `orders.order_request_id` is unique, so a second insert cannot land if the row lock is ever skipped. Product updates also require `stock >= quantity`, and the column itself cannot go negative.
- Prices are copied onto the request at submit time. A request that mixes currencies is rejected.
- Line rows store `workspace_id` and foreign keys back to the request and the product in that same workspace, so a line cannot point at another brand's product.
- Every read and write includes the workspace id from the header.
- The same buyer cannot submit the same lines twice while that request is still `new`. The match is the workspace, the email (ignoring case), and the products with their quantities. A partial unique index enforces it, including when two submits arrive together. After the request is accepted or rejected, that buyer can submit it again.

## Shortcuts, and what is next

- No authentication. The workspace header is the whole access check. The header dropdown exists so a demo can switch brands; a real app would use the signed-in brand instead.
- Pagination is limit/offset.
- `GET /api/order-requests/{id}` is extra. The sales page uses it to open one request, and a cross-workspace read is a 404 rather than an empty list.
- Product names on a request are read from the current catalog. There is no rename API.
- Accepted requests are not emailed. Next: log an acceptance email to the console.
- New requests do not expire. Next: a job that rejects requests still `new` after 7 days.
- The app is not deployed. Next: publish this Compose stack and send the URL.
