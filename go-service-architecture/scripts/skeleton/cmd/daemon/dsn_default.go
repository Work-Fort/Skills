//go:build !qa

package daemon

// resolveDSN returns the DSN unchanged in non-QA builds.
func resolveDSN(dsn string) string { return dsn }
