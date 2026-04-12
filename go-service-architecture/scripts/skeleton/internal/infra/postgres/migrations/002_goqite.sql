-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- +goose StatementBegin
CREATE FUNCTION goqite_update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
   NEW.updated = NOW();
   RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TABLE goqite (
    id       TEXT PRIMARY KEY DEFAULT ('m_' || encode(gen_random_bytes(16), 'hex')),
    created  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    queue    TEXT NOT NULL,
    body     BYTEA NOT NULL,
    timeout  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    received INTEGER NOT NULL DEFAULT 0,
    priority INTEGER NOT NULL DEFAULT 0
);

CREATE TRIGGER goqite_updated_timestamp
BEFORE UPDATE ON goqite
FOR EACH ROW EXECUTE FUNCTION goqite_update_timestamp();

CREATE INDEX goqite_queue_priority_created_idx ON goqite (queue, priority DESC, created);

-- +goose Down
DROP INDEX IF EXISTS goqite_queue_priority_created_idx;
DROP TRIGGER IF EXISTS goqite_updated_timestamp ON goqite;
DROP FUNCTION IF EXISTS goqite_update_timestamp();
DROP TABLE IF EXISTS goqite;
