package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/workfort/notifier/internal/domain"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

const requestIDKey contextKey = "request_id"

// WithRequestID generates a unique request ID, stores it in the
// request context, and sets the X-Request-ID response header.
func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := domain.NewID("req")
		w.Header().Set("X-Request-ID", reqID)
		ctx := context.WithValue(r.Context(), requestIDKey, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext extracts the request ID from the context.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.written = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.written = true
	return r.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter, required by
// http.ResponseController and middleware that need to access the
// original writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// WithRequestLogging logs every HTTP request with method, path,
// status code, and duration.
func WithRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		reqID := RequestIDFromContext(r.Context())
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start),
			"request_id", reqID,
		)
	})
}

// WithPanicRecovery catches panics in downstream handlers, logs the
// error, and returns HTTP 500 if no response has been written.
// This middleware is always wrapped by WithRequestLogging which
// installs a statusRecorder, so w is guaranteed to be a
// *statusRecorder here.
func WithPanicRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic recovered",
					"error", err,
					"method", r.Method,
					"path", r.URL.Path,
				)
				// Only write 500 if nothing has been sent yet.
				if rec, ok := w.(*statusRecorder); ok {
					if !rec.written {
						http.Error(w, "internal server error", http.StatusInternalServerError)
					}
				} else {
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// WithMiddleware applies the full middleware stack in the correct
// order (outermost first): request ID, then request logging, then
// panic recovery. Request ID is outermost so the logging middleware
// can read it from context. This extends observability spec REQ-008
// (which specifies logging then panic recovery) with the request ID
// layer; the spec should be updated to reflect this.
func WithMiddleware(next http.Handler) http.Handler {
	return WithRequestID(WithRequestLogging(WithPanicRecovery(next)))
}
