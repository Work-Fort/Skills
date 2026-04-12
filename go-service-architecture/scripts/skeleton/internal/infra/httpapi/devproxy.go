package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// NewSPADevProxy returns an HTTP handler that reverse-proxies all
// requests to the given development server URL (typically Vite on
// http://localhost:5173). Used during development for hot reload.
//
// Panics if devURL is not a valid URL. This follows the MustCompile
// convention — the URL is known at startup, so a parse failure is a
// programmer error.
func NewSPADevProxy(devURL string) http.Handler {
	target, err := url.Parse(devURL)
	if err != nil {
		panic(fmt.Sprintf("httpapi: invalid dev proxy URL %q: %v", devURL, err))
	}
	return httputil.NewSingleHostReverseProxy(target)
}
