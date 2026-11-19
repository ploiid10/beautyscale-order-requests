-- +goose Up
-- One open request per workspace, buyer email, and line list.
-- line_key is product ids and quantities, sorted, so line order does not matter.

ALTER TABLE order_requests ADD COLUMN line_key TEXT;

UPDATE order_requests AS r
SET line_key = COALESCE((
    SELECT string_agg(l.product_id::text || ':' || l.quantity::text, '|' ORDER BY l.product_id)
    FROM order_request_lines AS l
    WHERE l.order_request_id = r.id
), '');

ALTER TABLE order_requests ALTER COLUMN line_key SET NOT NULL;

-- Older copies of the same open request cannot stay new once the unique index exists.
WITH ranked AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY workspace_id, lower(buyer_email), line_key
               ORDER BY created_at DESC, id DESC
           ) AS n
    FROM order_requests
    WHERE status = 'new'
)
UPDATE order_requests
SET status = 'rejected'
WHERE id IN (SELECT id FROM ranked WHERE n > 1);

CREATE UNIQUE INDEX order_requests_open_same_idx
    ON order_requests (workspace_id, lower(buyer_email), line_key)
    WHERE status = 'new';

-- +goose Down
DROP INDEX IF EXISTS order_requests_open_same_idx;
ALTER TABLE order_requests DROP COLUMN IF EXISTS line_key;
