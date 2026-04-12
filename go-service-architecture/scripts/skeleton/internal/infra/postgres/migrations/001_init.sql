-- +goose Up
CREATE TABLE IF NOT EXISTS notifications (
    id          TEXT PRIMARY KEY,
    email       TEXT NOT NULL UNIQUE,
    status      INTEGER NOT NULL DEFAULT 0,
    retry_count INTEGER NOT NULL DEFAULT 0,
    retry_limit INTEGER NOT NULL DEFAULT 3,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_email ON notifications(email);
CREATE INDEX IF NOT EXISTS idx_notifications_status ON notifications(status);

-- +goose Down
DROP INDEX IF EXISTS idx_notifications_status;
DROP INDEX IF EXISTS idx_notifications_email;
DROP TABLE IF EXISTS notifications;
