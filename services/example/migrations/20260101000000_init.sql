-- +goose Up
CREATE TABLE widgets (
    id         UUID PRIMARY KEY,
    name       TEXT        NOT NULL,
    price_kop  BIGINT      NOT NULL CHECK (price_kop >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX widgets_created_at_idx ON widgets (created_at DESC);

-- +goose Down
DROP TABLE widgets;
