package queue

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"encoding/json"
	"testing"

	_ "modernc.org/sqlite"
)

func TestNotificationQueueEnqueue(t *testing.T) {
	db := setupTestDB(t)
	q, err := NewNotificationQueue(db, FlavorSQLite)
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

// TestQueueEnqueueDequeueIntegration exercises the full enqueue ->
// dequeue -> verify payload cycle using real goqite with a real SQLite
// database. Satisfies notification-delivery REQ-032.
func TestQueueEnqueueDequeueIntegration(t *testing.T) {
	db := setupTestDB(t)
	nq, err := NewNotificationQueue(db, FlavorSQLite)
	if err != nil {
		t.Fatalf("NewNotificationQueue() error: %v", err)
	}

	// Step 1: Create a payload and enqueue it.
	originalPayload := EmailJobPayload{
		NotificationID: "ntf_queue-int-1",
		Email:          "queue-int@test.com",
		RequestID:      "req_queue-int-1",
	}
	data, _ := json.Marshal(originalPayload)

	if err := nq.Enqueue(context.Background(), data); err != nil {
		t.Fatalf("Enqueue() error: %v", err)
	}

	// Step 2: Dequeue using goqite's Receive() directly.
	msg, err := nq.Queue().Receive(context.Background())
	if err != nil {
		t.Fatalf("Receive() error: %v", err)
	}
	if msg == nil {
		t.Fatal("Receive() returned nil message")
	}

	// Step 3: The received body is wrapped in a gob-encoded jobs envelope.
	// The envelope struct has Name (string) and Message ([]byte) fields.
	var envelope struct {
		Name    string
		Message []byte
	}
	if err := gob.NewDecoder(bytes.NewReader(msg.Body)).Decode(&envelope); err != nil {
		t.Fatalf("decode gob envelope: %v", err)
	}

	if envelope.Name != "send_notification" {
		t.Errorf("envelope name = %q, want send_notification", envelope.Name)
	}

	// Step 4: Verify the inner payload matches what was enqueued.
	var received EmailJobPayload
	if err := json.Unmarshal(envelope.Message, &received); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if received.NotificationID != originalPayload.NotificationID {
		t.Errorf("NotificationID = %q, want %q",
			received.NotificationID, originalPayload.NotificationID)
	}
	if received.Email != originalPayload.Email {
		t.Errorf("Email = %q, want %q", received.Email, originalPayload.Email)
	}
	if received.RequestID != originalPayload.RequestID {
		t.Errorf("RequestID = %q, want %q",
			received.RequestID, originalPayload.RequestID)
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
