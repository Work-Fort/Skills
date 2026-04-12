package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/workfort/notifier/internal/domain"
)

// testDSN reads the PostgreSQL DSN from the NOTIFIER_TEST_POSTGRES_DSN
// environment variable. Tests are skipped if not set.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("NOTIFIER_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("NOTIFIER_TEST_POSTGRES_DSN not set; skipping PostgreSQL tests")
	}
	return dsn
}

func TestPostgresOpenAndPing(t *testing.T) {
	dsn := testDSN(t)
	store, err := Open(dsn)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer func() { _ = store.Close() }()

	if err := store.Ping(context.Background()); err != nil {
		t.Errorf("Ping() error: %v", err)
	}
}

func TestPostgresCreateAndGetNotification(t *testing.T) {
	dsn := testDSN(t)
	store, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Clean up from previous test runs.
	_, _ = store.db.ExecContext(ctx, "DELETE FROM notifications WHERE email = $1", "pgtest@test.com")

	n := &domain.Notification{
		ID:         "ntf_pg-001",
		Email:      "pgtest@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: domain.DefaultRetryLimit,
	}

	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatalf("CreateNotification() error: %v", err)
	}

	got, err := store.GetNotificationByEmail(ctx, "pgtest@test.com")
	if err != nil {
		t.Fatalf("GetNotificationByEmail() error: %v", err)
	}
	if got.ID != n.ID {
		t.Errorf("ID = %q, want %q", got.ID, n.ID)
	}
	if got.Email != n.Email {
		t.Errorf("Email = %q, want %q", got.Email, n.Email)
	}
	if got.Status != domain.StatusPending {
		t.Errorf("Status = %v, want %v", got.Status, domain.StatusPending)
	}
}

func TestPostgresCreateDuplicateReturnsAlreadyNotified(t *testing.T) {
	dsn := testDSN(t)
	store, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	_, _ = store.db.ExecContext(ctx, "DELETE FROM notifications WHERE email = $1", "pgdup@test.com")

	n := &domain.Notification{
		ID:         "ntf_pgdup-1",
		Email:      "pgdup@test.com",
		Status:     domain.StatusPending,
		RetryLimit: domain.DefaultRetryLimit,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	n2 := &domain.Notification{
		ID:         "ntf_pgdup-2",
		Email:      "pgdup@test.com",
		Status:     domain.StatusPending,
		RetryLimit: domain.DefaultRetryLimit,
	}
	err = store.CreateNotification(ctx, n2)
	if err == nil {
		t.Fatal("expected error for duplicate email, got nil")
	}
	if !errors.Is(err, domain.ErrAlreadyNotified) {
		t.Errorf("error = %v, want ErrAlreadyNotified", err)
	}
}

func TestPostgresGetNotificationNotFound(t *testing.T) {
	dsn := testDSN(t)
	store, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	_, err = store.GetNotificationByEmail(context.Background(), "pgnobody@test.com")
	if err == nil {
		t.Fatal("expected error for missing notification, got nil")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestPostgresListNotifications(t *testing.T) {
	dsn := testDSN(t)
	store, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	// Clean up.
	_, _ = store.db.ExecContext(ctx, "DELETE FROM notifications")

	for _, email := range []string{"pga@test.com", "pgb@test.com", "pgc@test.com"} {
		n := &domain.Notification{
			ID:         domain.NewID("ntf"),
			Email:      email,
			Status:     domain.StatusPending,
			RetryLimit: domain.DefaultRetryLimit,
		}
		if err := store.CreateNotification(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	list, err := store.ListNotifications(ctx, "", 2)
	if err != nil {
		t.Fatalf("ListNotifications() error: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("len = %d, want 2", len(list))
	}

	// Second page.
	list2, err := store.ListNotifications(ctx, list[1].ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(list2) != 1 {
		t.Errorf("page 2 len = %d, want 1", len(list2))
	}
}

func TestPostgresLogTransition(t *testing.T) {
	dsn := testDSN(t)
	store, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	if err := store.LogTransition(ctx, "notification", "ntf_pglog-1",
		domain.StatusPending, domain.StatusSending, domain.TriggerSend); err != nil {
		t.Fatalf("LogTransition() error: %v", err)
	}

	var count int
	err = store.db.QueryRowContext(ctx,
		"SELECT count(*) FROM state_transitions WHERE entity_id = $1",
		"ntf_pglog-1",
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("transition count = %d, want 1", count)
	}
}

// Compile-time check: Store implements domain.Store.
var _ domain.Store = (*Store)(nil)
