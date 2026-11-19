-- +goose Up
-- Money is integer minor units. Stock cannot go negative.
-- Line tables carry workspace_id so a line cannot point at another brand's product.

CREATE TABLE workspaces (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE products (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspaces (id),
    name TEXT NOT NULL,
    price_minor BIGINT NOT NULL CHECK (price_minor >= 0),
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    stock INTEGER NOT NULL CHECK (stock >= 0),
    UNIQUE (id, workspace_id)
);

CREATE INDEX products_workspace_id_idx ON products (workspace_id);

CREATE TABLE order_requests (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspaces (id),
    buyer_name TEXT NOT NULL,
    buyer_email TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('new', 'accepted', 'rejected')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (id, workspace_id)
);

CREATE INDEX order_requests_workspace_status_created_idx
    ON order_requests (workspace_id, status, created_at DESC);

CREATE TABLE order_request_lines (
    id UUID PRIMARY KEY,
    order_request_id UUID NOT NULL,
    workspace_id UUID NOT NULL,
    product_id UUID NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_price_minor BIGINT NOT NULL CHECK (unit_price_minor >= 0),
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    UNIQUE (order_request_id, product_id),
    FOREIGN KEY (order_request_id, workspace_id)
        REFERENCES order_requests (id, workspace_id),
    FOREIGN KEY (product_id, workspace_id)
        REFERENCES products (id, workspace_id)
);

CREATE INDEX order_request_lines_request_idx
    ON order_request_lines (order_request_id);

CREATE TABLE orders (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL,
    order_request_id UUID NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status = 'draft'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (id, workspace_id),
    FOREIGN KEY (order_request_id, workspace_id)
        REFERENCES order_requests (id, workspace_id)
);

CREATE INDEX orders_workspace_id_idx ON orders (workspace_id);

CREATE TABLE order_lines (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL,
    workspace_id UUID NOT NULL,
    product_id UUID NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_price_minor BIGINT NOT NULL CHECK (unit_price_minor >= 0),
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    FOREIGN KEY (order_id, workspace_id)
        REFERENCES orders (id, workspace_id),
    FOREIGN KEY (product_id, workspace_id)
        REFERENCES products (id, workspace_id)
);

CREATE INDEX order_lines_order_id_idx ON order_lines (order_id);

-- +goose Down
DROP TABLE IF EXISTS order_lines;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS order_request_lines;
DROP TABLE IF EXISTS order_requests;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS workspaces;
