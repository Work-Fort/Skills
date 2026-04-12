-- +goose Up
CREATE TABLE IF NOT EXISTS state_transitions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    from_state  TEXT NOT NULL,
    to_state    TEXT NOT NULL,
    trigger     TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_state_transitions_entity
    ON state_transitions(entity_type, entity_id);

-- +goose Down
DROP INDEX IF EXISTS idx_state_transitions_entity;
DROP TABLE IF EXISTS state_transitions;
