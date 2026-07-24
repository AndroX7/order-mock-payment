CREATE TABLE uploads (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id     uuid        NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    filename     text        NOT NULL,
    content_type text        NOT NULL,
    size         bigint      NOT NULL CHECK (size > 0),
    path         text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX uploads_order_id_idx ON uploads (order_id);
