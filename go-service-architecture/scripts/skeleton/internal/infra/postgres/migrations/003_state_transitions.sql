-- +goose Up
CREATE TABLE IF NOT EXISTS state_transitions (
    id          SERIAL PRIMARY KEY,
    entity_type TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    from_state  TEXT NOT NULL,
    to_state    TEXT NOT NULL,
    trigger     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_state_transitions_entity
    ON state_transitions(entity_type, entity_id);

-- +goose Down
DROP INDEX IF EXISTS idx_state_transitions_entity;
DROP TABLE IF EXISTS state_transitions;
