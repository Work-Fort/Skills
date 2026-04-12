package postgres

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/qmuntal/stateless"

	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx" driver

	"github.com/workfort/notifier/internal/domain"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store implements domain.Store backed by PostgreSQL.
type Store struct {
	db *sql.DB
}

// DB returns the underlying *sql.DB for sharing with goqite
// (service-database REQ-019).
func (s *Store) DB() *sql.DB {
	return s.db
}

// Open creates a new PostgreSQL store. The DSN must be a valid
// PostgreSQL connection string (e.g.,
// "postgres://user:pass@localhost:5432/notifydb?sslmode=disable").
// Migrations are run automatically on open.
func Open(dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	// REQ-010: connection pool settings.
	db.SetMaxOpenConns(25)
	// REQ-011: idle connections.
	db.SetMaxIdleConns(5)
	// REQ-012: connection lifetime.
	db.SetConnMaxLifetime(5 * time.Minute)

	// Verify connectivity before running migrations.
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	// Run embedded goose migrations.
	goose.SetLogger(goose.NopLogger())
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("postgres"); err != nil {
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
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		n.ID, n.Email, int(n.Status), n.RetryCount, n.RetryLimit, now, now,
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
		 FROM notifications WHERE email = $1`, email,
	)

	n := &domain.Notification{}
	var status int
	err := row.Scan(&n.ID, &n.Email, &status, &n.RetryCount, &n.RetryLimit, &n.CreatedAt, &n.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("get notification %s: %w", email, domain.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get notification: %w", err)
	}
	n.Status = domain.Status(status)
	return n, nil
}

// UpdateNotification updates an existing notification record.
func (s *Store) UpdateNotification(ctx context.Context, n *domain.Notification) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET status = $1, retry_count = $2, retry_limit = $3, updated_at = $4
		 WHERE id = $5`,
		int(n.Status), n.RetryCount, n.RetryLimit, now, n.ID,
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
			 FROM notifications ORDER BY created_at ASC, id ASC LIMIT $1`, limit,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, email, status, retry_count, retry_limit, created_at, updated_at
			 FROM notifications
			 WHERE (created_at, id) > (
			     SELECT created_at, id FROM notifications WHERE id = $1
			 )
			 ORDER BY created_at ASC, id ASC LIMIT $2`, after, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []*domain.Notification
	for rows.Next() {
		n := &domain.Notification{}
		var status int
		if err := rows.Scan(&n.ID, &n.Email, &status, &n.RetryCount, &n.RetryLimit, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		n.Status = domain.Status(status)
		result = append(result, n)
	}
	return result, rows.Err()
}

// CountNotifications returns the total number of notification records.
// Satisfies notification-management REQ-020.
func (s *Store) CountNotifications(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notifications").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count notifications: %w", err)
	}
	return count, nil
}

// NotificationStateAccessor returns an accessor function for the
// stateless state machine. It reads the current status of a
// notification by ID from the database.
func (s *Store) NotificationStateAccessor(notificationID string) func(ctx context.Context) (stateless.State, error) {
	return func(ctx context.Context) (stateless.State, error) {
		var state int
		err := s.db.QueryRowContext(ctx,
			"SELECT status FROM notifications WHERE id = $1", notificationID,
		).Scan(&state)
		if err != nil {
			return nil, fmt.Errorf("read notification state: %w", err)
		}
		return domain.Status(state), nil
	}
}

// NotificationStateMutator returns a mutator function for the
// stateless state machine. It writes the new status to the database
// and updates the updated_at timestamp.
func (s *Store) NotificationStateMutator(notificationID string) func(ctx context.Context, state stateless.State) error {
	return func(ctx context.Context, state stateless.State) error {
		now := time.Now().UTC()
		_, err := s.db.ExecContext(ctx,
			"UPDATE notifications SET status = $1, updated_at = $2 WHERE id = $3",
			int(state.(domain.Status)), now, notificationID,
		)
		if err != nil {
			return fmt.Errorf("write notification state: %w", err)
		}
		return nil
	}
}

// LogTransition records a state transition in the audit log table.
// Stores human-readable string values.
func (s *Store) LogTransition(ctx context.Context, entityType, entityID string, from, to domain.Status, trigger domain.Trigger) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		entityType, entityID, from.String(), to.String(), trigger.String(),
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("log transition: %w", err)
	}
	return nil
}

// isUniqueViolation checks if a PostgreSQL error is a UNIQUE constraint
// violation. pgx returns errors containing "duplicate key value
// violates unique constraint" or SQLSTATE code 23505.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23505") ||
		strings.Contains(msg, "duplicate key value violates unique constraint")
}
