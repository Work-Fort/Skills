package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/workfort/notifier/internal/domain"
)

// HandleHealth returns an http.HandlerFunc that checks database
// connectivity via domain.HealthChecker.
func HandleHealth(checker domain.HealthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := checker.Ping(r.Context())
		status := "healthy"
		httpCode := http.StatusOK
		if err != nil {
			status = "unhealthy"
			httpCode = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpCode)
		//nolint:errcheck // response write errors are unactionable after WriteHeader
		json.NewEncoder(w).Encode(map[string]string{"status": status})
	}
}
