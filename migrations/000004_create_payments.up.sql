CREATE TABLE payments (
    id                 uuid           PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id           uuid           NOT NULL UNIQUE REFERENCES orders(id) ON DELETE CASCADE,
    provider           text           NOT NULL,
    provider_reference text           NOT NULL,
    amount             numeric(24, 8) NOT NULL CHECK (amount > 0),
    currency           text           NOT NULL,
    status             text           NOT NULL DEFAULT 'pending',
    created_at         timestamptz    NOT NULL DEFAULT now(),
    updated_at         timestamptz    NOT NULL DEFAULT now()
);
