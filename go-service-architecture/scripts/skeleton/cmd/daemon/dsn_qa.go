//go:build qa

package daemon

// resolveDSN forces in-memory SQLite in QA builds so the database
// is fresh on every startup and seed data always runs cleanly.
func resolveDSN(_ string) string { return ":memory:" }
