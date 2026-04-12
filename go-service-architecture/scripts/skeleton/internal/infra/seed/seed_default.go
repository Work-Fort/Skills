//go:build !qa

package seed

import "database/sql"

// RunSeed is a no-op in non-QA builds.
func RunSeed(_ *sql.DB, _ ...any) error {
	return nil
}
