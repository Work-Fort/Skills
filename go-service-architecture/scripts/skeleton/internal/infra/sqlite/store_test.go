package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/workfort/notifier/internal/domain"
)

func TestOpenInMemory(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatalf("Open(\"\") error: %v", err)
	}
	defer func() { _ = store.Close() }()
}

func TestPing(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	if err := store.Ping(context.Background()); err != nil {
		t.Errorf("Ping() error: %v", err)
	}
}

func TestCreateAndGetNotification(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_test-123",
		Email:      "test@test.com",
		Status:     domain.StatusPending,
		RetryCount: 0,
		RetryLimit: domain.DefaultRetryLimit,
	}

	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatalf("CreateNotification() error: %v", err)
	}

	got, err := store.GetNotificationByEmail(ctx, "test@test.com")
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
	if got.RetryLimit != domain.DefaultRetryLimit {
		t.Errorf("RetryLimit = %d, want %d", got.RetryLimit, domain.DefaultRetryLimit)
	}
}

func TestCreateDuplicateReturnsAlreadyNotified(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_dup-1",
		Email:      "dup@test.com",
		Status:     domain.StatusPending,
		RetryLimit: domain.DefaultRetryLimit,
	}

	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	n2 := &domain.Notification{
		ID:         "ntf_dup-2",
		Email:      "dup@test.com",
		Status:     domain.StatusPending,
		RetryLimit: domain.DefaultRetryLimit,
	}
	err = store.CreateNotification(ctx, n2)
	if err == nil {
		t.Fatal("expected error for duplicate email, got nil")
	}
	if !isDomainErr(err, domain.ErrAlreadyNotified) {
		t.Errorf("error = %v, want ErrAlreadyNotified", err)
	}
}

func TestGetNotificationNotFound(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	_, err = store.GetNotificationByEmail(context.Background(), "nobody@test.com")
	if err == nil {
		t.Fatal("expected error for missing notification, got nil")
	}
	if !isDomainErr(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestUpdateNotification(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	n := &domain.Notification{
		ID:         "ntf_upd-1",
		Email:      "update@test.com",
		Status:     domain.StatusPending,
		RetryLimit: domain.DefaultRetryLimit,
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}

	n.Status = domain.StatusSending
	if err := store.UpdateNotification(ctx, n); err != nil {
		t.Fatalf("UpdateNotification() error: %v", err)
	}

	got, err := store.GetNotificationByEmail(ctx, "update@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusSending {
		t.Errorf("Status = %v, want %v", got.Status, domain.StatusSending)
	}
}

func TestListNotifications(t *testing.T) {
	store, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	for i, email := range []string{"a@test.com", "b@test.com", "c@test.com"} {
		n := &domain.Notification{
			ID:         domain.NewID("ntf"),
			Email:      email,
			Status:     domain.StatusPending,
			RetryLimit: domain.DefaultRetryLimit,
		}
		_ = i
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

	// Second page using the last ID as cursor.
	list2, err := store.ListNotifications(ctx, list[1].ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(list2) != 1 {
		t.Errorf("page 2 len = %d, want 1", len(list2))
	}
}

// isDomainErr is a test helper using errors.Is.
func isDomainErr(err, target error) bool {
	return err != nil && errors.Is(err, target)
}
