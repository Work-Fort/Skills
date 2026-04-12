package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	_ "modernc.org/sqlite"
)

func TestNotificationQueueEnqueue(t *testing.T) {
	db := setupTestDB(t)
	q, err := NewNotificationQueue(db)
	if err != nil {
		t.Fatalf("NewNotificationQueue() error: %v", err)
	}

	payload := map[string]string{"email": "test@company.com"}
	data, _ := json.Marshal(payload)

	if err := q.Enqueue(context.Background(), data); err != nil {
		t.Fatalf("Enqueue() error: %v", err)
	}

	// Verify a message exists in the goqite table.
	var count int
	err = db.QueryRow("SELECT count(*) FROM goqite WHERE queue = 'notifications'").Scan(&count)
	if err != nil {
		t.Fatalf("query goqite: %v", err)
	}
	if count != 1 {
		t.Errorf("goqite message count = %d, want 1", count)
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Create the goqite schema (same as migration 002).
	schema := `
	create table goqite (
		id text primary key default ('m_' || lower(hex(randomblob(16)))),
		created text not null default (strftime('%Y-%m-%dT%H:%M:%fZ')),
		updated text not null default (strftime('%Y-%m-%dT%H:%M:%fZ')),
		queue text not null,
		body blob not null,
		timeout text not null default (strftime('%Y-%m-%dT%H:%M:%fZ')),
		received integer not null default 0,
		priority integer not null default 0
	) strict;

	create trigger goqite_updated_timestamp after update on goqite begin
		update goqite set updated = strftime('%Y-%m-%dT%H:%M:%fZ') where id = old.id;
	end;

	create index goqite_queue_priority_created_idx on goqite (queue, priority desc, created);`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create goqite schema: %v", err)
	}
	return db
}
