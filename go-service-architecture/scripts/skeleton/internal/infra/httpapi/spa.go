package httpapi

import (
	"io/fs"
	"net/http"
	"strings"
)

// NewSPAHandler serves static files from the given filesystem. If a
// requested path does not match a real file, it falls back to
// index.html for client-side routing. Files under assets/ receive
// immutable cache headers since Vite content-hashes their filenames.
func NewSPAHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "index.html"
		} else if len(path) > 0 && path[0] == '/' {
			path = path[1:]
		}

		// Serve the file if it exists.
		if _, err := fs.Stat(fsys, path); err == nil {
			// Hashed assets get long-lived cache headers.
			if strings.HasPrefix(path, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// Fallback: serve index.html for client-side routing.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
