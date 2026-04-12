//go:build qa

package seed

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"maragu.dev/goqite"
	"maragu.dev/goqite/jobs"
)

//go:embed testdata/seed.sql
var seedSQL []byte

// SeedSQL returns the raw embedded seed SQL. Exposed for testing.
func SeedSQL() []byte {
	return seedSQL
}

// seedJobs defines the goqite jobs to enqueue for seed notifications.
// Only pending and not_sent notifications need delivery jobs -- terminal
// states (delivered, failed) do not get jobs.
var seedJobs = []struct {
	NotificationID string `json:"notification_id"`
	Email          string `json:"email"`
	RequestID      string `json:"request_id"`
}{
	// Step 3: pending notifications.
	{"ntf_seed-001", "alice@company.com", "req_seed-001"},
	{"ntf_seed-002", "bob@company.com", "req_seed-002"},
	{"ntf_seed-003", "charlie@example.com", "req_seed-003"},
	// Step 4: not_sent notification (will auto-retry).
	{"ntf_seed-006", "retry@company.com", "req_seed-006"},
}

// RunSeed executes the embedded seed SQL and enqueues delivery jobs.
// The sqlFlavor parameter selects the goqite SQL dialect (0 for SQLite,
// use goqite.SQLFlavorPostgreSQL for PostgreSQL).
func RunSeed(db *sql.DB, sqlFlavor ...goqite.SQLFlavor) error {
	if _, err := db.Exec(string(seedSQL)); err != nil {
		return fmt.Errorf("run seed sql: %w", err)
	}

	opts := goqite.NewOpts{
		DB:         db,
		Name:       "notifications",
		MaxReceive: 8,
		Timeout:    30 * time.Second,
	}
	if len(sqlFlavor) > 0 {
		opts.SQLFlavor = sqlFlavor[0]
	}
	q := goqite.New(opts)

	ctx := context.Background()
	for _, sj := range seedJobs {
		payload, err := json.Marshal(sj)
		if err != nil {
			return fmt.Errorf("marshal seed job %s: %w", sj.NotificationID, err)
		}
		if _, err := jobs.Create(ctx, q, "send_notification", goqite.Message{Body: payload}); err != nil {
			return fmt.Errorf("enqueue seed job %s: %w", sj.NotificationID, err)
		}
	}

	return nil
}
