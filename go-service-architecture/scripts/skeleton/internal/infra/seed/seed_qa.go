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
// Each entry corresponds to a notification row inserted by seed.sql.
var seedJobs = []struct {
	NotificationID string `json:"notification_id"`
	Email          string `json:"email"`
	RequestID      string `json:"request_id"`
}{
	{"ntf_seed-001", "alice@company.com", "req_seed-001"},
	{"ntf_seed-002", "bob@company.com", "req_seed-002"},
	{"ntf_seed-003", "charlie@example.com", "req_seed-003"},
}

// RunSeed executes the embedded seed SQL and enqueues delivery jobs
// programmatically via jobs.Create(). Called on startup when built
// with -tags qa.
func RunSeed(db *sql.DB) error {
	// Insert notification rows via SQL.
	if _, err := db.Exec(string(seedSQL)); err != nil {
		return fmt.Errorf("run seed sql: %w", err)
	}

	// Enqueue delivery jobs via jobs.Create() so the gob-encoded
	// envelope format matches what the Runner expects.
	q := goqite.New(goqite.NewOpts{
		DB:         db,
		Name:       "notifications",
		MaxReceive: 8,
		Timeout:    30 * time.Second,
	})

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
