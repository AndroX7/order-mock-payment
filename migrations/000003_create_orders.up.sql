CREATE TABLE orders (
    id         uuid           PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid           NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    symbol     text           NOT NULL,
    side       text           NOT NULL CHECK (side IN ('BUY', 'SELL')),
    quantity   numeric(24, 8) NOT NULL CHECK (quantity > 0),
    price      numeric(24, 8) NOT NULL CHECK (price >= 0),
    status     text           NOT NULL DEFAULT 'pending',
    created_at timestamptz    NOT NULL DEFAULT now(),
    updated_at timestamptz    NOT NULL DEFAULT now()
);

CREATE INDEX orders_user_id_created_at_idx ON orders (user_id, created_at DESC);
