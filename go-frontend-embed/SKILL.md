---
name: go-frontend-embed
description: Embed frontend assets into a Go binary for single-binary deployment. Use when adding a web UI to a Go service, embedding Vite/React/SPA build output, serving static assets from Go, or setting up dev proxy to a frontend dev server. Use when user says "embed frontend", "single binary with UI", "serve React from Go", "SPA handler", or "dev proxy to Vite".
license: MIT
metadata:
  author: Kaz Walker
  version: "1.0"
---

# Go Frontend Embed

Embed a frontend SPA (React, Vue, Svelte, etc.) into a Go binary so the
service ships as a single executable with web assets included.

**Core pattern:** Build tags control whether the binary includes real
assets or an empty filesystem. Production builds use `-tags spa` after
compiling the frontend. Development uses a reverse proxy to the frontend
dev server.

## Directory Layout

```
web/
  src/              -- frontend source (React, etc.)
  dist/             -- Vite build output (gitignored)
  package.json
  vite.config.ts
cmd/
  web/
    embed.go        -- empty embed (no build tag)
    embed_spa.go    -- real embed (build tag: spa)
    web.go          -- subcommand wiring
internal/
  infra/
    httpapi/
      spa.go        -- SPA handler + dev proxy
```

## Conditional Embed with Build Tags

Two files in the same package — only one compiles based on the `spa`
build tag.

**`cmd/web/embed.go`** — default (development):

```go
//go:build !spa

package web

import "embed"

// webFS is empty when built without the "spa" tag.
// Use --dev to proxy to Vite's dev server during development.
var webFS embed.FS
```

**`cmd/web/embed_spa.go`** — production:

```go
//go:build spa

package web

import "embed"

// webFS holds the Vite build output. Built via:
//   cd web && npm run build && go build -tags spa
//
//go:embed all:dist
var webFS embed.FS
```

The `all:` prefix includes dotfiles and files normally excluded by Go's
embed rules. Use it to ensure nothing from the Vite build is silently
dropped.

## SPA Handler

Serves static files from the embedded filesystem. Routes that don't
match a real file fall back to `index.html` for client-side routing.

```go
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
```

Vite content-hashes filenames under `assets/` — these are immutable and
safe to cache for a year. Everything else gets default caching.

## Dev Proxy

During development, proxy requests to Vite's dev server for hot reload:

```go
func NewSPADevProxy(devURL string) http.Handler {
    target, _ := url.Parse(devURL)
    return httputil.NewSingleHostReverseProxy(target)
}
```

## Wiring It Together

In the subcommand that starts the HTTP server:

```go
func run(cmd *cobra.Command, args []string) error {
    var spaHandler http.Handler
    if dev {
        spaHandler = httpapi.NewSPADevProxy(devURL)
    } else {
        distFS, err := fs.Sub(webFS, "dist")
        if err != nil {
            return fmt.Errorf("embedded SPA: %w", err)
        }
        spaHandler = httpapi.NewSPAHandler(distFS)
    }

    mux := http.NewServeMux()
    // API routes first — they take priority over the SPA catch-all.
    mux.HandleFunc("GET /v1/health", handleHealth(store))
    registerEntityRoutes(api, store)

    // SPA catch-all last.
    mux.Handle("/", spaHandler)

    srv := &http.Server{Addr: addr, Handler: mux}
    return srv.ListenAndServe()
}
```

API routes are registered before the SPA catch-all so they always take
priority. The SPA handler only receives requests that don't match an API
route.

## Build Integration

### Mise Task Files

Tasks live in `.mise/tasks/` as executable bash scripts. Subdirectories
create colon-separated namespaces (see go-service-architecture skill).

```
.mise/
  tasks/
    build/
      web             -- mise run build:web
      go              -- mise run build:go
    release/
      dev             -- mise run release:dev
      production      -- mise run release:production
    lint/
      web             -- mise run lint:web
    clean/
      web             -- mise run clean:web
    dev/
      web             -- mise run dev:web
```

**`.mise/tasks/build/web`:**

```bash
#!/usr/bin/env bash
#MISE description="Build frontend"
set -euo pipefail

cd web
npm run build
```

**`.mise/tasks/build/go`:**

```bash
#!/usr/bin/env bash
#MISE description="Build Go binary (without SPA)"
set -euo pipefail

go build -o build/myservice .
```

**`.mise/tasks/release/dev`:**

```bash
#!/usr/bin/env bash
#MISE description="Build debug binary with race detector (no SPA)"
set -euo pipefail

go build -race -o build/myservice .
```

**`.mise/tasks/release/production`:**

```bash
#!/usr/bin/env bash
#MISE description="Build release binary with embedded frontend"
#MISE depends=["build:web"]
set -euo pipefail

VERSION="${VERSION:-dev}"
CGO_ENABLED=0 go build -tags spa \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -trimpath \
    -o build/myservice .
```

**`.mise/tasks/dev/web`:**

```bash
#!/usr/bin/env bash
#MISE description="Start Vite dev server"
set -euo pipefail

cd web
npm run dev
```

**`.mise/tasks/lint/web`:**

```bash
#!/usr/bin/env bash
#MISE description="Lint frontend"
set -euo pipefail

cd web
npm run lint
```

**`.mise/tasks/clean/web`:**

```bash
#!/usr/bin/env bash
#MISE description="Remove frontend build artifacts"
set -euo pipefail

rm -rf web/dist
```

### Production build

```bash
mise run release:production
```

### Development

Terminal 1: `mise run dev:web` (Vite dev server on :5173)
Terminal 2: `go run . web --dev --dev-url http://localhost:5173`

## React + Vite Setup

Minimal setup for a React SPA that builds to `web/dist/`:

```bash
cd web
npm create vite@latest . -- --template react-ts
npm install
```

**`web/vite.config.ts`:**

```typescript
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
```

The default Vite config hashes asset filenames under `dist/assets/`,
which aligns with the cache headers in the SPA handler above.

## Anti-Patterns

- **Embedding `node_modules/` or source files.** Only embed the build
  output (`dist/`). The `embed_spa.go` directive should point at `dist`
  only.
- **Forgetting `fs.Sub`.** The embedded filesystem is rooted at
  `cmd/web/dist/`. Use `fs.Sub(webFS, "dist")` to strip the prefix
  before passing to the handler.
- **API routes after the SPA catch-all.** The SPA handler matches
  everything — register API routes first.
- **Caching `index.html`.** Only hash-named files under `assets/` should
  get immutable cache headers. `index.html` references the current
  asset hashes and must not be cached.
