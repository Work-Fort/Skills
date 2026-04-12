package db

import (
	"context"
	"database/sql"
	"strings"

	"github.com/qmuntal/stateless"

	"github.com/workfort/notifier/internal/domain"
	"github.com/workfort/notifier/internal/infra/postgres"
	"github.com/workfort/notifier/internal/infra/sqlite"
)

// backendStore extends domain.Store with the state machine
// accessor/mutator and DB() method that both sqlite.Store and
// postgres.Store implement.
type backendStore interface {
	domain.Store
	DB() *sql.DB
	NotificationStateAccessor(notificationID string) func(ctx context.Context) (stateless.State, error)
	NotificationStateMutator(notificationID string) func(ctx context.Context, state stateless.State) error
}

// Store wraps either a SQLite or PostgreSQL backend behind the
// backendStore interface. All domain.Store methods delegate through
// the interface field, so adding new methods to domain.Store only
// requires updating the concrete stores -- the dispatcher stays
// unchanged.
type Store struct {
	store backendStore
	db    *sql.DB
}

// Open selects the database backend based on DSN prefix and returns
// a Store that satisfies domain.Store and exposes DB().
//
// - DSN starting with "postgres" selects PostgreSQL (REQ-001).
// - All other values (including empty) select SQLite (REQ-001).
func Open(dsn string) (*Store, error) {
	if strings.HasPrefix(dsn, "postgres") {
		pg, err := postgres.Open(dsn)
		if err != nil {
			return nil, err
		}
		return &Store{store: pg, db: pg.DB()}, nil
	}
	sq, err := sqlite.Open(dsn)
	if err != nil {
		return nil, err
	}
	return &Store{store: sq, db: sq.DB()}, nil
}

// DB returns the underlying *sql.DB for sharing with goqite
// (service-database REQ-019).
func (s *Store) DB() *sql.DB {
	return s.db
}

// --- domain.Store delegation (all through s.store) ---

func (s *Store) Ping(ctx context.Context) error {
	return s.store.Ping(ctx)
}

func (s *Store) CreateNotification(ctx context.Context, n *domain.Notification) error {
	return s.store.CreateNotification(ctx, n)
}

func (s *Store) GetNotificationByEmail(ctx context.Context, email string) (*domain.Notification, error) {
	return s.store.GetNotificationByEmail(ctx, email)
}

func (s *Store) UpdateNotification(ctx context.Context, n *domain.Notification) error {
	return s.store.UpdateNotification(ctx, n)
}

func (s *Store) ListNotifications(ctx context.Context, after string, limit int) ([]*domain.Notification, error) {
	return s.store.ListNotifications(ctx, after, limit)
}

func (s *Store) CountNotifications(ctx context.Context) (int, error) {
	return s.store.CountNotifications(ctx)
}

func (s *Store) NotificationStateAccessor(notificationID string) func(ctx context.Context) (stateless.State, error) {
	return s.store.NotificationStateAccessor(notificationID)
}

func (s *Store) NotificationStateMutator(notificationID string) func(ctx context.Context, state stateless.State) error {
	return s.store.NotificationStateMutator(notificationID)
}

func (s *Store) LogTransition(ctx context.Context, entityType, entityID string, from, to domain.Status, trigger domain.Trigger) error {
	return s.store.LogTransition(ctx, entityType, entityID, from, to, trigger)
}

func (s *Store) Close() error {
	return s.store.Close()
}
