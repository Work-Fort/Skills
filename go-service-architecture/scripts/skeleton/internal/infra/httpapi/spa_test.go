package httpapi

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestNewSPAHandler_ServesIndexHTML(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<html>app</html>")},
	}

	handler := NewSPAHandler(fsys)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "<html>app</html>" {
		t.Fatalf("expected index.html content, got %q", body)
	}
}

func TestNewSPAHandler_FallbackToIndex(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<html>app</html>")},
	}

	handler := NewSPAHandler(fsys)
	req := httptest.NewRequest(http.MethodGet, "/notifications/ntf_abc123", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "<html>app</html>" {
		t.Fatalf("expected index.html fallback, got %q", body)
	}
}

func TestNewSPAHandler_ServesStaticFile(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":             {Data: []byte("<html>app</html>")},
		"assets/index-abc123.js": {Data: []byte("console.log('app')")},
	}

	handler := NewSPAHandler(fsys)
	req := httptest.NewRequest(http.MethodGet, "/assets/index-abc123.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "console.log('app')" {
		t.Fatalf("expected JS content, got %q", body)
	}
}

func TestNewSPAHandler_ImmutableCacheForAssets(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":             {Data: []byte("<html>app</html>")},
		"assets/index-abc123.js": {Data: []byte("console.log('app')")},
	}

	handler := NewSPAHandler(fsys)
	req := httptest.NewRequest(http.MethodGet, "/assets/index-abc123.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	cc := rec.Header().Get("Cache-Control")
	expected := "public, max-age=31536000, immutable"
	if cc != expected {
		t.Fatalf("expected Cache-Control %q, got %q", expected, cc)
	}
}

func TestNewSPAHandler_NoCacheForIndex(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<html>app</html>")},
	}

	handler := NewSPAHandler(fsys)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	cc := rec.Header().Get("Cache-Control")
	if cc == "public, max-age=31536000, immutable" {
		t.Fatal("index.html must not receive immutable cache headers")
	}
}

func TestNewSPAHandler_FallbackNoCacheHeaders(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<html>app</html>")},
	}

	handler := NewSPAHandler(fsys)
	req := httptest.NewRequest(http.MethodGet, "/notifications/ntf_abc123", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if body := rec.Body.String(); body != "<html>app</html>" {
		t.Fatalf("expected index.html fallback, got %q", body)
	}
	cc := rec.Header().Get("Cache-Control")
	if cc == "public, max-age=31536000, immutable" {
		t.Fatal("fallback to index.html must not receive immutable cache headers")
	}
}

// Ensure the function signature accepts fs.FS so it works with
// the output of fs.Sub(webFS, "dist").
var _ = func(fsys fs.FS) http.Handler {
	return NewSPAHandler(fsys)
}
