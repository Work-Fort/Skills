package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestIDMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request ID is in the context.
		reqID := RequestIDFromContext(r.Context())
		if reqID == "" {
			t.Error("request ID not found in context")
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := WithRequestID(inner)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify X-Request-ID header is set and has a reasonable format
	// (prefix + separator + UUID). The exact prefix is an implementation
	// detail tested in domain/identity_test.go.
	rid := rec.Header().Get("X-Request-ID")
	if rid == "" {
		t.Fatal("X-Request-ID response header is empty")
	}
	if !strings.Contains(rid, "_") {
		t.Errorf("X-Request-ID = %q, want format prefix_uuid", rid)
	}
}

func TestRequestLoggingMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	handler := WithRequestLogging(inner)
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusTeapot)
	}
}

func TestPanicRecoveryMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	handler := WithPanicRecovery(inner)
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()

	// Should not panic.
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestStatusRecorderUnwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, status: http.StatusOK}
	if sr.Unwrap() != rec {
		t.Error("Unwrap() did not return the underlying ResponseWriter")
	}
}

func TestMiddlewareStackOrder(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request ID is available (set by outer middleware).
		if RequestIDFromContext(r.Context()) == "" {
			t.Error("request ID not set by middleware stack")
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := WithMiddleware(inner)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("X-Request-ID header missing from middleware stack")
	}
}
