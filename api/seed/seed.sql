-- Idempotent demo data. Re-running does not reset stock or duplicate rows.
-- Lumen Atelier:      8f1c0b2e-6a4d-4c1e-9b7a-1d2e3f4a5b6c
-- Northline Beauty:   3c9e1a70-2b54-4f8d-a6e1-7b8c9d0e1f2a

INSERT INTO workspaces (id, name) VALUES
    ('8f1c0b2e-6a4d-4c1e-9b7a-1d2e3f4a5b6c', 'Lumen Atelier'),
    ('3c9e1a70-2b54-4f8d-a6e1-7b8c9d0e1f2a', 'Northline Beauty')
ON CONFLICT (id) DO NOTHING;

INSERT INTO products (id, workspace_id, name, price_minor, currency, stock) VALUES
    ('5b1e2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d', '8f1c0b2e-6a4d-4c1e-9b7a-1d2e3f4a5b6c', 'Rose Serum', 4800, 'USD', 20),
    ('6c2f3d4e-5f6a-4b7c-8d9e-0f1a2b3c4d5e', '8f1c0b2e-6a4d-4c1e-9b7a-1d2e3f4a5b6c', 'Silk Cleanser', 3200, 'USD', 15),
    ('7d3a4e5f-6a7b-4c8d-9e0f-1a2b3c4d5e6f', '8f1c0b2e-6a4d-4c1e-9b7a-1d2e3f4a5b6c', 'Night Oil', 6400, 'USD', 8),
    ('1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d', '3c9e1a70-2b54-4f8d-a6e1-7b8c9d0e1f2a', 'Glacier Balm', 2500, 'EUR', 30),
    ('2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e', '3c9e1a70-2b54-4f8d-a6e1-7b8c9d0e1f2a', 'Birch Toner', 1800, 'EUR', 12),
    ('3c4d5e6f-7a8b-4c9d-8e1f-2a3b4c5d6e70', '3c9e1a70-2b54-4f8d-a6e1-7b8c9d0e1f2a', 'Mineral Mist', 2200, 'EUR', 4)
ON CONFLICT (id) DO NOTHING;
