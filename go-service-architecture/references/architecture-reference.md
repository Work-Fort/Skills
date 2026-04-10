# Architecture Reference

Full implementation examples for go-service-architecture. Load this when
writing or reviewing Go service code.

---

## Domain Layer

### Types

`internal/domain/types.go` — entity structs, enums, and constants. No
external imports beyond stdlib.

```go
package domain

import "time"

type Status string

const (
    StatusActive   Status = "active"
    StatusInactive Status = "inactive"
)

type MyEntity struct {
    ID        string    `json:"id"`
    Name      string    `json:"name"`
    Status    Status    `json:"status"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

### Error Sentinels

`internal/domain/errors.go` — domain errors as package-level variables:

```go
var (
    ErrNotFound         = errors.New("not found")
    ErrAlreadyExists    = errors.New("already exists")
    ErrHasDependencies  = errors.New("has dependencies")
    ErrPermissionDenied = errors.New("permission denied")
)
```

Callers check with `errors.Is(err, domain.ErrNotFound)`. Infra layers
wrap with context: `fmt.Errorf("get entity %s: %w", id, err)`.

Go 1.26 introduced `errors.AsType[T](err)` as a type-safe alternative
to `errors.As` for extracting structured error types:

```go
if myErr, ok := errors.AsType[*MyError](err); ok {
    // use myErr
}
```

### Port Interfaces

Split port definitions across files by concern:

```
internal/domain/
  types.go           -- entity structs, enums
  errors.go          -- error sentinels
  store.go           -- EntityStore, HealthChecker, Store interfaces
  identity.go        -- IdentityProvider interface
  chat.go            -- ChatProvider interface
  gitforge.go        -- GitForgeProvider interface
```

The package stays `domain` — do not split into sub-packages
(`domain/store/`, `domain/identity/`) as this creates circular import
risks when entity types and port interfaces reference each other.

```go
// Small, focused interfaces — consumers accept only what they need.
type EntityStore interface {
    CreateEntity(ctx context.Context, e *MyEntity) error
    GetEntity(ctx context.Context, id string) (*MyEntity, error)
    ListEntities(ctx context.Context) ([]*MyEntity, error)
    UpdateEntity(ctx context.Context, e *MyEntity) error
    DeleteEntity(ctx context.Context, id string) error
}

type HealthChecker interface {
    Ping(ctx context.Context) error
}

// Store combines all storage interfaces for use at the composition root.
// Consumers (handlers, services) accept individual interfaces, not Store.
type Store interface {
    EntityStore
    HealthChecker
    io.Closer
}
```

---

## Service Layer

For simple CRUD, handlers call domain ports directly:

    handler → EntityStore

When business logic emerges, introduce a service type:

    handler → Service → EntityStore

```go
type EntityService struct {
    store  EntityStore
    auth   AuthProvider  // another port
}

func (s *EntityService) Create(ctx context.Context, e *MyEntity) error {
    // business logic here — validation, authorization, side effects
    return s.store.CreateEntity(ctx, e)
}
```

The service lives in `internal/service/` or `internal/domain/`. It depends
only on domain ports, never on infrastructure.

---

## CLI and Config

### Root Command

```go
var Version = "dev" // set via -ldflags at build time

func newRootCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:     "myservice",
        Version: Version,
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
            config.InitDirs()
            return config.LoadConfig()
        },
    }
    cmd.AddCommand(daemon.NewCmd(), mcpbridge.NewCmd(), admin.NewCmd())
    return cmd
}

func Execute() {
    if err := newRootCmd().Execute(); err != nil {
        os.Exit(1)
    }
}
```

### Config

Koanf loads from: config file, then env vars (later sources win).

```go
import (
    "strings"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/koanf/providers/env"
    "github.com/knadh/koanf/providers/file"
    "github.com/knadh/koanf/parsers/yaml"
)

var k = koanf.New(".")

func LoadConfig(configPath string) error {
    // Load from YAML file (missing file is ok)
    _ = k.Load(file.Provider(configPath), yaml.Parser())
    // Load from environment — strip prefix, lowercase, replace _ with .
    if err := k.Load(env.Provider("MYSERVICE_", ".", func(s string) string {
        return strings.Replace(
            strings.ToLower(strings.TrimPrefix(s, "MYSERVICE_")),
            "_", ".", -1,
        )
    }), nil); err != nil {
        return fmt.Errorf("load env config: %w", err)
    }
    return nil
}
```

Access values via `k.String("port")`, `k.Int("port")`, etc.

Koanf instances are not goroutine-safe for concurrent Load/Get. The
package-level instance is safe when loaded once during startup and read
during request handling. If hot reload is needed, wrap with `sync.RWMutex`.

XDG-compliant paths: `$XDG_CONFIG_HOME/<service>/config.yaml` for config,
`$XDG_STATE_HOME/<service>/` for runtime state (logs, DB).

---

## Database Layer

### SQLite Store

```go
//go:embed migrations/*.sql
var migrations embed.FS

func Open(dsn string) (*Store, error) {
    db, err := sql.Open("sqlite", dsn)
    if err != nil {
        return nil, err
    }
    db.SetMaxOpenConns(1) // SQLite requires single-writer serialization
    if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
        return nil, fmt.Errorf("set WAL mode: %w", err)
    }
    if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
        return nil, fmt.Errorf("enable foreign keys: %w", err)
    }
    if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
        return nil, fmt.Errorf("set busy timeout: %w", err)
    }
    if err := goose.SetBaseFS(migrations); err != nil {
        return nil, fmt.Errorf("set migration fs: %w", err)
    }
    if err := goose.SetDialect("sqlite3"); err != nil {
        return nil, fmt.Errorf("set dialect: %w", err)
    }
    if err := goose.Up(db, "migrations"); err != nil {
        return nil, fmt.Errorf("run migrations: %w", err)
    }
    return &Store{db: db}, nil
}
```

- In-memory store for tests: `Open("")`
- Migrations are numbered SQL files: `001_init.sql`, `002_feature.sql`
- `-- +goose Up` / `-- +goose Down` markers
- Parameterized `?` placeholders — never string concatenation

### PostgreSQL Store

Same port interface, different implementation. Uses `pgx/v5` connection
pool. Migrations use PostgreSQL syntax where needed.

```go
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(5 * time.Minute)
```

### Dual Backend

A top-level dispatcher selects based on DSN:

```go
func Open(dsn string) (domain.Store, error) {
    if strings.HasPrefix(dsn, "postgres") {
        return postgres.Open(dsn)
    }
    return sqlite.Open(dsn)
}
```

---

## Background Queue

Use `maragu.dev/goqite` for persistent, database-backed background task
queues. Supports SQLite and PostgreSQL via `SQLFlavor`, shares the same
`*sql.DB` as the store.

### Setup

Install the schema via a goose migration:

```sql
-- +goose Up
-- contents of goqite's schema_sqlite.sql or schema_postgres.sql
-- +goose Down
DROP TABLE IF EXISTS goqite;
```

Initialise at startup:

```go
q := goqite.New(goqite.NewOpts{
    DB:         db,
    Name:       "callbacks",
    MaxReceive: 3,
    Timeout:    10 * time.Second,
    // SQLFlavor: goqite.SQLFlavorPostgreSQL, // uncomment for Postgres
})
```

### Job Runner

```go
r := jobs.NewRunner(jobs.NewRunnerOpts{
    Limit:        5,
    Log:          slog.Default(),
    PollInterval: 500 * time.Millisecond,
    Queue:        q,
})

r.Register("servicenow_callback", func(ctx context.Context, payload []byte) error {
    return nil
})

if err := jobs.Create(ctx, q, "servicenow_callback", payload); err != nil {
    return fmt.Errorf("enqueue callback: %w", err)
}

r.Start(ctx)
```

---

## HTTP Server

### Server Setup

```go
type ServerConfig struct {
    Bind  string
    Port  int
    Store domain.Store
}

func NewServer(cfg ServerConfig) *http.Server {
    mux := http.NewServeMux()
    mux.HandleFunc("GET /v1/health", handleHealth(cfg.Store))

    api := humago.New(mux, huma.DefaultConfig("My Service", Version))
    registerEntityRoutes(api, cfg.Store)

    mcpHandler := newMCPHandler(cfg.Store)
    mux.Handle("/mcp", mcpHandler)

    handler := withMiddleware(mux)

    return &http.Server{
        Addr:              fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port),
        Handler:           handler,
        ReadTimeout:       15 * time.Second,
        WriteTimeout:      15 * time.Second,
        IdleTimeout:       60 * time.Second,
        ReadHeaderTimeout: 5 * time.Second,
    }
}
```

For endpoints that accept request bodies, limit size:

```go
r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
```

### REST Route Registration

One `register*Routes` function per resource type:

```go
func registerEntityRoutes(api huma.API, store domain.Store) {
    huma.Register(api, huma.Operation{
        Method:      http.MethodGet,
        Path:        "/v1/entities",
        OperationID: "list-entities",
    }, func(ctx context.Context, input *struct{}) (*EntityListOutput, error) {
        list, err := store.ListEntities(ctx)
        if err != nil {
            return nil, mapDomainErr(err)
        }
        return &EntityListOutput{Body: list}, nil
    })
}
```

### Error Mapping

```go
func mapDomainErr(err error) error {
    switch {
    case errors.Is(err, domain.ErrNotFound):
        return huma.Error404NotFound("not found")
    case errors.Is(err, domain.ErrAlreadyExists):
        return huma.Error409Conflict("already exists")
    case errors.Is(err, domain.ErrPermissionDenied):
        return huma.Error403Forbidden("forbidden")
    default:
        slog.Error("unhandled domain error", "error", err)
        return huma.Error500InternalServerError("internal error")
    }
}
```

---

## Middleware

```go
func withMiddleware(next http.Handler) http.Handler {
    return withRequestLogging(withPanicRecovery(next))
}

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

func (r *statusRecorder) Unwrap() http.ResponseWriter {
    return r.ResponseWriter
}

func withRequestLogging(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
        next.ServeHTTP(rec, r)
        slog.Info("request",
            "method", r.Method,
            "path", r.URL.Path,
            "status", rec.status,
            "duration", time.Since(start),
        )
    })
}

func withPanicRecovery(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if rec := recover(); rec != nil {
                slog.Error("panic recovered", "error", rec)
                if rw, ok := w.(*statusRecorder); ok && !rw.written {
                    http.Error(w, "internal server error", http.StatusInternalServerError)
                }
            }
        }()
        next.ServeHTTP(w, r)
    })
}
```

---

## MCP Integration

### Server Side

Mount a `StreamableHTTPServer` at `/mcp`:

```go
func newMCPHandler(store domain.Store) http.Handler {
    s := server.NewMCPServer("myservice", Version)
    s.AddTool(mcp.NewTool("list_entities",
        mcp.WithDescription("List all entities"),
    ), handleListEntities(store))
    return server.NewStreamableHTTPServer(s)
}
```

When mounting on an external mux, use `http.StripPrefix`:

```go
mux.Handle("/mcp/", http.StripPrefix("/mcp", mcpHandler))
```

### Bridge Subcommand

`cmd/mcpbridge/` — reads JSON-RPC from stdin, forwards to
`http://<host>:<port>/mcp`, relays responses to stdout. Passes auth
token on every request.

---

## Health Endpoint

```go
func handleHealth(store domain.Store) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        err := store.Ping(r.Context())
        status := "healthy"
        httpCode := 200
        if err != nil {
            status = "unhealthy"
            httpCode = 503
        }
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(httpCode)
        //nolint:errcheck // response write errors are unactionable after WriteHeader
        json.NewEncoder(w).Encode(map[string]string{"status": status})
    }
}
```

---

## Daemon Subcommand

```go
func NewCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "daemon",
        Short: "Start the HTTP server",
        RunE:  run,
    }
    cmd.Flags().String("bind", "127.0.0.1", "Bind address")
    cmd.Flags().Int("port", 8080, "Listen port")
    cmd.Flags().String("db", "", "Database path")
    return cmd
}

func run(cmd *cobra.Command, args []string) error {
    store, err := infra.Open(k.String("db"))
    if err != nil {
        return err
    }
    defer store.Close()

    srv := daemon.NewServer(daemon.ServerConfig{
        Bind:  k.String("bind"),
        Port:  k.Int("port"),
        Store: store,
    })

    // graceful shutdown
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()
    errCh := make(chan error, 1)
    go func() {
        if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
            errCh <- err
        }
    }()
    select {
    case <-ctx.Done():
    case err := <-errCh:
        return err
    }
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()
    return srv.Shutdown(shutdownCtx)
}
```

If the service uses MCP with SSE streaming, shut down the
`StreamableHTTPServer` separately:

```go
mcpHandler.Shutdown(shutdownCtx) // signal SSE clients to reconnect
```

### Graceful Shutdown with Background Queue

Stop the job runner before closing the store:

```go
shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
defer cancel()
srv.Shutdown(shutdownCtx)
runnerCancel() // cancels the context passed to r.Start
store.Close()
```

---

## Observability

- **Structured logging**: `log/slog` — JSON output via `slog.NewJSONHandler`; attach request IDs via context.
- **Metrics**: `prometheus/client_golang` — expose `/metrics` for Prometheus scraping.
- **Tracing**: `go.opentelemetry.io/otel` — instrument handlers and store calls with spans.

---

## ID Generation

```go
func NewID(prefix string) string {
    return fmt.Sprintf("%s_%s", prefix, uuid.New().String())
}
```

---

## Testing

- Standard library `testing` — no testify, no mock frameworks
- Table-driven tests with `t.Run` subtests
- In-memory SQLite for store tests: `sqlite.Open("")`
- `net/http/httptest` for handler tests
- Stub implementations of port interfaces for service logic tests
- Test files alongside source (`_test.go` suffix)
- E2E tests in a separate `tests/` directory with its own `go.mod`

---

## Build Tooling

```toml
[tools]
go = "1.26.0"

[tasks.build]
description = "Build the binary"
run = "go build -o build/myservice ."

[tasks.test]
description = "Run unit tests"
run = "go test ./..."

[tasks.lint]
description = "Run linter"
run = "golangci-lint run ./..."

[tasks.clean]
description = "Remove build artifacts"
run = "rm -rf build/"
```

---

## Dockerfile

Multi-stage build with distroless runtime:

```dockerfile
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o build/myservice .

FROM gcr.io/distroless/static-debian12
COPY --from=build /src/build/myservice /usr/local/bin/myservice
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/myservice"]
CMD ["daemon"]
```

`modernc.org/sqlite` is CGO-free, so `distroless/static` works without libc.
If a CGO dependency is required, switch to `gcr.io/distroless/base-debian12`.

---

## Emerging Patterns

**json/v2 (experimental since Go 1.25)**
Case-sensitive by default, faster unmarshaling. Enable with `GOEXPERIMENT=jsonv2`.

**eBPF-based OpenTelemetry auto-instrumentation (beta)**
Zero-code-change tracing via eBPF hooks. Still beta — monitor for GA.

**Bounded context nesting**
For multi-domain services: `internal/chat/domain/`, `internal/flow/domain/`.
Not needed for single-context services.

**Vertical slice organization**
Organizing by feature rather than by layer. Complements hexagonal for early stages.

**errors.AsType[T] (Go 1.26)**
Type-safe generic alternative to `errors.As`. Reduces boilerplate for structured error types.

---

## Further Reading

- [ThreeDotsLabs — DDD, CQRS, and Clean Architecture in Go](https://threedots.tech/post/ddd-cqrs-clean-architecture-combined/)
- [Wild Workouts](https://github.com/ThreeDotsLabs/wild-workouts-go-ddd-example) — full DDD example
- [Watermill](https://watermill.io/) — event-driven Go library
- [Go Official Module Layout](https://go.dev/doc/modules/layout)
- [Huma Framework](https://huma.rocks/)
- [koanf](https://github.com/knadh/koanf)
- [OpenTelemetry Go SDK](https://opentelemetry.io/docs/languages/go/)
- [Go 1.26 Release Notes](https://go.dev/doc/go1.26)
