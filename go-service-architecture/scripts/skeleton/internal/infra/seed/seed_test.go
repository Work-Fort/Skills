//go:build qa

package seed

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSeedSQLNotEmpty(t *testing.T) {
	data := SeedSQL()
	if len(data) == 0 {
		t.Fatal("SeedSQL() returned empty bytes in qa build")
	}
}

func TestRunSeedExecutes(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create the tables that seed data depends on.
	schema := `
	CREATE TABLE IF NOT EXISTS notifications (
		id          TEXT PRIMARY KEY,
		email       TEXT NOT NULL UNIQUE,
		status      INTEGER NOT NULL DEFAULT 0,
		retry_count INTEGER NOT NULL DEFAULT 0,
		retry_limit INTEGER NOT NULL DEFAULT 3,
		created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
		updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
	);
	CREATE TABLE IF NOT EXISTS state_transitions (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		entity_type TEXT NOT NULL,
		entity_id   TEXT NOT NULL,
		from_state  TEXT NOT NULL,
		to_state    TEXT NOT NULL,
		trigger     TEXT NOT NULL,
		created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
	);
	create table goqite (
		id text primary key default ('m_' || lower(hex(randomblob(16)))),
		created text not null default (strftime('%Y-%m-%dT%H:%M:%fZ')),
		updated text not null default (strftime('%Y-%m-%dT%H:%M:%fZ')),
		queue text not null,
		body blob not null,
		timeout text not null default (strftime('%Y-%m-%dT%H:%M:%fZ')),
		received integer not null default 0,
		priority integer not null default 0
	) strict;`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	if err := RunSeed(db); err != nil {
		t.Fatalf("RunSeed() error: %v", err)
	}

	// Verify seed data was inserted.
	var count int
	if err := db.QueryRow("SELECT count(*) FROM notifications").Scan(&count); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if count != 6 {
		t.Errorf("notification count = %d, want 6", count)
	}

	// Verify queue jobs were enqueued.
	if err := db.QueryRow("SELECT count(*) FROM goqite").Scan(&count); err != nil {
		t.Fatalf("count goqite: %v", err)
	}
	if count != 4 {
		t.Errorf("goqite message count = %d, want 4", count)
	}
}
