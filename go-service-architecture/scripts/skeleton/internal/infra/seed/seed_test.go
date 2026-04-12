//go:build qa

package seed

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSeedSQLNotEmpty(t *testing.T) {
	data := SeedSQL()
	if len(data) == 0 {
		t.Fatal("SeedSQL() returned empty bytes in qa build")
	}
}

func TestRunSeedExecutes(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// The placeholder seed SQL is just a comment, so it should
	// succeed on any database without requiring schema.
	if err := RunSeed(db); err != nil {
		t.Fatalf("RunSeed() error: %v", err)
	}
}
