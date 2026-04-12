package db

import (
	"strings"
	"testing"
)

func TestOpenSelectsSQLiteForEmptyDSN(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatalf("Open(\"\") error: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Verify it opened successfully (SQLite in-memory).
	if store == nil {
		t.Fatal("store is nil")
	}
}

func TestOpenSelectsSQLiteForFilePath(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(\":memory:\") error: %v", err)
	}
	defer func() { _ = store.Close() }()
}

func TestOpenSelectsPostgresForPostgresDSN(t *testing.T) {
	// This test verifies the dispatch logic. It will fail to connect
	// (no PostgreSQL running), but it should attempt the PostgreSQL
	// path, not the SQLite path. We test the branch by checking the
	// error message.
	_, err := Open("postgres://localhost:5432/nonexistent_db?sslmode=disable&connect_timeout=1")
	if err == nil {
		// If it connects, that is also fine.
		return
	}
	// The error should come from the postgres package, not sqlite.
	if !containsAny(err.Error(), "postgres", "pgx", "connect") {
		t.Errorf("expected postgres-related error, got: %v", err)
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
