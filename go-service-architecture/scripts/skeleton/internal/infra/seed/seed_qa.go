//go:build qa

package seed

import (
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed testdata/seed.sql
var seedSQL []byte

// SeedSQL returns the raw embedded seed SQL. Exposed for testing.
func SeedSQL() []byte {
	return seedSQL
}

// RunSeed executes the embedded seed SQL against the given database.
// Called on startup when built with -tags qa.
func RunSeed(db *sql.DB) error {
	if _, err := db.Exec(string(seedSQL)); err != nil {
		return fmt.Errorf("run seed: %w", err)
	}
	return nil
}
