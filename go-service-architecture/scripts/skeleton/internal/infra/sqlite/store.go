package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/workfort/notifier/internal/domain"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store implements domain.Store backed by SQLite.
type Store struct {
	db *sql.DB
}

// DB returns the underlying *sql.DB for sharing with goqite.
// Satisfies service-database REQ-019.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Open creates a new SQLite store. An empty DSN creates an in-memory
// database (for tests). A non-empty DSN is used as a file path.
// Migrations are run automatically.
func Open(dsn string) (*Store, error) {
	if dsn == "" {
		dsn = ":memory:"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// REQ-007: single-writer serialization.
	db.SetMaxOpenConns(1)

	// REQ-004: WAL mode.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	// REQ-005: foreign keys.
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	// REQ-006: busy timeout.
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	// Run embedded goose migrations.
	goose.SetLogger(goose.NopLogger())
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return nil, fmt.Errorf("set dialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Ping verifies the database connection is alive.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// CreateNotification inserts a new notification record. Returns
// domain.ErrAlreadyNotified if the email already exists.
func (s *Store) CreateNotification(ctx context.Context, n *domain.Notification) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notifications (id, email, status, retry_count, retry_limit, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.Email, int(n.Status), n.RetryCount, n.RetryLimit,
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("create notification %s: %w", n.Email, domain.ErrAlreadyNotified)
		}
		return fmt.Errorf("create notification: %w", err)
	}
	n.CreatedAt = now
	n.UpdatedAt = now
	return nil
}

// GetNotificationByEmail retrieves a notification by email address.
// Returns domain.ErrNotFound if no record exists.
func (s *Store) GetNotificationByEmail(ctx context.Context, email string) (*domain.Notification, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, status, retry_count, retry_limit, created_at, updated_at
		 FROM notifications WHERE email = ?`, email,
	)

	n := &domain.Notification{}
	var status int
	var createdAt, updatedAt string
	err := row.Scan(&n.ID, &n.Email, &status, &n.RetryCount, &n.RetryLimit, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("get notification %s: %w", email, domain.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get notification: %w", err)
	}
	n.Status = domain.Status(status)
	n.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	n.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return n, nil
}

// UpdateNotification updates an existing notification record.
func (s *Store) UpdateNotification(ctx context.Context, n *domain.Notification) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET status = ?, retry_count = ?, retry_limit = ?, updated_at = ?
		 WHERE id = ?`,
		int(n.Status), n.RetryCount, n.RetryLimit, now.Format(time.RFC3339), n.ID,
	)
	if err != nil {
		return fmt.Errorf("update notification: %w", err)
	}
	n.UpdatedAt = now
	return nil
}

// ListNotifications returns notifications with cursor-based pagination.
// If after is empty, returns from the beginning. Limit controls page size.
func (s *Store) ListNotifications(ctx context.Context, after string, limit int) ([]*domain.Notification, error) {
	var rows *sql.Rows
	var err error
	if after == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, email, status, retry_count, retry_limit, created_at, updated_at
			 FROM notifications ORDER BY created_at ASC, id ASC LIMIT ?`, limit,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, email, status, retry_count, retry_limit, created_at, updated_at
			 FROM notifications
			 WHERE (created_at, id) > (
			     SELECT created_at, id FROM notifications WHERE id = ?
			 )
			 ORDER BY created_at ASC, id ASC LIMIT ?`, after, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	var result []*domain.Notification
	for rows.Next() {
		n := &domain.Notification{}
		var status int
		var createdAt, updatedAt string
		if err := rows.Scan(&n.ID, &n.Email, &status, &n.RetryCount, &n.RetryLimit, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		n.Status = domain.Status(status)
		n.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		n.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		result = append(result, n)
	}
	return result, rows.Err()
}

// isUniqueViolation checks if a SQLite error is a UNIQUE constraint
// violation. modernc.org/sqlite returns error strings containing
// "UNIQUE constraint failed".
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
