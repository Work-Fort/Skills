# Architecture Reference

Full implementation examples for go-service-architecture. Load this when
writing or reviewing Go service code.

---

## Domain Layer

### Types

`internal/domain/types.go` — entity structs, enums, and constants. No
external imports beyond stdlib.

Note: `type Status string` is for simple status fields on non-state-machine
entities. For state machine states, use `type Status int` with iota enums
(see the State Machines section below).

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

func NewRootCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:     "myservice",
        Version: Version,
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
            if err := config.InitDirs(); err != nil {
                return err
            }
            return config.Load()
        },
        // Silence Cobra's built-in error/usage printing so we
        // control output via os.Exit in Execute().
        SilenceUsage:  true,
        SilenceErrors: true,
    }
    cmd.AddCommand(daemon.NewCmd(), mcpbridge.NewCmd())
    return cmd
}

func Execute() {
    if err := NewRootCmd().Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
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

func Load() error {
    // Reset for testability — allows calling Load() multiple times
    // in tests without accumulating state from prior calls.
    K = koanf.New(".")

    // Load from YAML file (missing file is ok)
    _ = K.Load(file.Provider(ConfigPath()), yaml.Parser())
    // Load from environment — strip prefix, lowercase, replace _ with .
    if err := K.Load(env.Provider("MYSERVICE_", ".", func(s string) string {
        return strings.ReplaceAll(
            strings.ToLower(strings.TrimPrefix(s, "MYSERVICE_")),
            "_", ".",
        )
    }), nil); err != nil {
        return fmt.Errorf("load env config: %w", err)
    }
    return nil
}
```

Access values via `K.String("port")`, `K.Int("port")`, etc.

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
    goose.SetLogger(goose.NopLogger()) // suppress migration output
    goose.SetBaseFS(migrations)
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

## State Machines and Workflows

Three tiers depending on complexity. Choose the simplest that fits.

### Tier 1: Hand-Rolled Transition Map

For simple state machines (3-5 states, no guards, no visualization):

```go
type OrderState int

const (
    OrderDraft OrderState = iota
    OrderSubmitted
    OrderApproved
    OrderRejected
)

type OrderEvent int

const (
    EventSubmit OrderEvent = iota
    EventApprove
    EventReject
)

var orderTransitions = map[OrderState]map[OrderEvent]OrderState{
    OrderDraft:     {EventSubmit: OrderSubmitted},
    OrderSubmitted: {EventApprove: OrderApproved, EventReject: OrderRejected},
}

func (o *Order) Apply(event OrderEvent) error {
    targets, ok := orderTransitions[o.State]
    if !ok {
        return fmt.Errorf("no transitions from state %d", o.State)
    }
    next, ok := targets[event]
    if !ok {
        return fmt.Errorf("event %d not valid in state %d", event, o.State)
    }
    o.State = next
    return nil
}
```

Zero dependencies, compile-time type safety with iota enums, trivially
testable. Migrate to `stateless` later if complexity grows — the domain
types (state enums, transition rules) transfer directly.

### Tier 2: stateless Library

For complex transitions with guards, callbacks, persistence, and
visualization. Use `qmuntal/stateless` (BSD-2-Clause, zero external
dependencies, stdlib only).

#### Domain Definition

State machine configuration lives in the domain layer. States and
triggers are iota enums:

```go
package domain

import "github.com/qmuntal/stateless"

type TicketState int

const (
    TicketOpen TicketState = iota
    TicketInProgress
    TicketReview
    TicketResolved
    TicketClosed
)

type TicketTrigger int

const (
    TriggerAssign TicketTrigger = iota
    TriggerSubmitForReview
    TriggerApprove
    TriggerReject
    TriggerClose
    TriggerReopen
)

func ConfigureTicketMachine(
    accessor func(ctx context.Context) (stateless.State, error),
    mutator func(ctx context.Context, state stateless.State) error,
) *stateless.StateMachine {
    sm := stateless.NewStateMachineWithExternalStorage(accessor, mutator, stateless.FiringQueued)

    sm.Configure(TicketOpen).
        Permit(TriggerAssign, TicketInProgress)

    sm.Configure(TicketInProgress).
        Permit(TriggerSubmitForReview, TicketReview).
        Permit(TriggerClose, TicketClosed)

    sm.Configure(TicketReview).
        Permit(TriggerApprove, TicketResolved).
        Permit(TriggerReject, TicketInProgress).
        OnEntry(func(ctx context.Context, args ...any) error {
            // notify reviewer
            return nil
        })

    sm.Configure(TicketResolved).
        Permit(TriggerClose, TicketClosed).
        Permit(TriggerReopen, TicketOpen)

    sm.Configure(TicketClosed).
        Permit(TriggerReopen, TicketOpen)

    return sm
}
```

The `accessor` and `mutator` are plain functions — no infra imports in
the domain. The infra layer provides concrete implementations.

#### Guards

Guards are predicates that must be true for a transition to fire:

```go
sm.Configure(TicketReview).
    PermitIf(TriggerApprove, TicketResolved,
        func(ctx context.Context, args ...any) bool {
            return args[0].(*Ticket).TestsPassing
        },
    )
```

Guards within a state must be mutually exclusive.

#### Firing Transitions

Use `FireCtx` (not `Fire`) to propagate context through transitions:

```go
err := sm.FireCtx(ctx, TriggerApprove)
```

`Fire(trigger, args...)` does NOT accept a context as its first
argument — passing one will be interpreted as the trigger value and
cause a runtime error. Always use `FireCtx(ctx, trigger, args...)`
when context propagation is needed (which is almost always).

#### Infra Adapter (Persistence)

```go
func (s *Store) TicketStateAccessor(ticketID string) func(ctx context.Context) (stateless.State, error) {
    return func(ctx context.Context) (stateless.State, error) {
        var state int
        err := s.db.QueryRowContext(ctx,
            "SELECT state FROM tickets WHERE id = ?", ticketID,
        ).Scan(&state)
        return stateless.State(state), err
    }
}

func (s *Store) TicketStateMutator(ticketID string) func(ctx context.Context, state stateless.State) error {
    return func(ctx context.Context, state stateless.State) error {
        _, err := s.db.ExecContext(ctx,
            "UPDATE tickets SET state = ?, updated_at = ? WHERE id = ?",
            int(state.(TicketState)), time.Now().UTC(), ticketID,
        )
        return err
    }
}
```

#### Transition Audit Log (Optional)

For audit trails, log transitions in the store:

```go
_, err = s.db.ExecContext(ctx,
    `INSERT INTO state_transitions (entity_type, entity_id, from_state, to_state, trigger, actor_id, created_at)
     VALUES (?, ?, ?, ?, ?, ?, ?)`,
    "ticket", ticketID, fromState, toState, trigger, actorID, time.Now().UTC(),
)
```

#### Visualization

Export to Graphviz DOT format for documentation:

```go
graph := sm.ToGraph()
// renders DOT format — pipe to `dot -Tsvg` for visual
```

### Tier 3: Workflow Engine

For complex business processes with directed graphs, role-based
permissions, integration hooks, and work item tracking. This goes
beyond a single entity's state machine into orchestrating multi-step
processes across teams and systems.

A workflow engine provides three layers:

- **Templates** — process blueprints defining steps (graph nodes),
  transitions (graph edges), role permissions, and integration hooks
- **Instances** — bind a template to a team and its integrations
  (git forge, chat, identity provider)
- **Work items** — flow through an instance, moving between steps as
  agents or humans trigger transitions

```
Template (blueprint)
  |
  +-- Instance (bound to team + integrations)
        |
        +-- Work Item --> [Step A] --transition--> [Step B] --> ...
```

#### Guard Expressions with CEL

Use `google/cel-go` (already in the library stack) for transition guard
expressions. CEL is non-Turing-complete, safe to evaluate with
untrusted input, and used by Kubernetes and Google Cloud IAM:

```go
// Guard expression stored as a string in the template
guard := `assignee.role == "reviewer" && item.fields.tests_passing == true`

// Evaluate at transition time
env, _ := cel.NewEnv(
    cel.Variable("assignee", cel.ObjectType("Agent")),
    cel.Variable("item", cel.ObjectType("WorkItem")),
)
ast, _ := env.Compile(guard)
prg, _ := env.Program(ast)
out, _, _ := prg.Eval(map[string]any{
    "assignee": assignee,
    "item":     workItem,
})
if out.Value().(bool) {
    // transition allowed
}
```

#### When to Use Each Tier

| Signals | Tier |
|---------|------|
| Single entity, few states, no guards | Hand-rolled map |
| Single entity, complex transitions, guards, persistence | `stateless` |
| Multi-step process, multiple roles, integrations, work item tracking | Workflow engine |

Most services start with hand-rolled and grow into `stateless`. A
workflow engine is a separate service, not a library added to an
existing service.

---

## Email

### Port Interface

```go
// EmailSender sends email messages. Implementations live in infra/.
type EmailSender interface {
    Send(ctx context.Context, msg *EmailMessage) error
}

type EmailMessage struct {
    To      []string
    Subject string
    HTML    string
    Text    string
}
```

Handlers and services accept `EmailSender`, not a concrete SMTP client.

### SMTP Adapter

Use `wneessen/go-mail` for SMTP sending. It supports STARTTLS, implicit
TLS, and XOAUTH2 (required by Gmail and Microsoft). CGO-free, stdlib
dependencies only.

For local development (e.g., Mailpit on port 1025), use no auth and
`TLSOpportunistic`. Production deployments would add
`gomail.WithSMTPAuth`, `gomail.WithUsername`, `gomail.WithPassword`,
and `gomail.WithTLSPolicy(gomail.TLSMandatory)`.

```go
import gomail "github.com/wneessen/go-mail"

type SMTPSender struct {
    client *gomail.Client
    from   string
}

func NewSMTPSender(host string, port int, from string) (*SMTPSender, error) {
    c, err := gomail.NewClient(host,
        gomail.WithPort(port),
        gomail.WithTLSPolicy(gomail.TLSOpportunistic),
    )
    if err != nil {
        return nil, fmt.Errorf("create smtp client: %w", err)
    }
    return &SMTPSender{client: c, from: from}, nil
}

func (s *SMTPSender) Send(ctx context.Context, msg *EmailMessage) error {
    m := gomail.NewMsg()
    if err := m.From(s.from); err != nil {
        return fmt.Errorf("set from: %w", err)
    }
    if err := m.To(msg.To...); err != nil {
        return fmt.Errorf("set to: %w", err)
    }
    m.Subject(msg.Subject)
    m.SetBodyString(gomail.TypeTextHTML, msg.HTML)
    m.AddAlternativeString(gomail.TypeTextPlain, msg.Text)

    // Propagate X-Request-ID into email headers for traceability.
    if msg.RequestID != "" {
        m.SetGenHeader(gomail.Header("X-Request-ID"), msg.RequestID)
    }

    return s.client.DialAndSend(m)
}
```

Do not send email synchronously in HTTP handlers. Enqueue via goqite
and send from a background worker:

```go
// In the handler or service:
jobs.Create(ctx, q, "send_email", emailPayload)

// In the job runner:
r.Register("send_email", func(ctx context.Context, payload []byte) error {
    var msg EmailMessage
    json.Unmarshal(payload, &msg)
    return sender.Send(ctx, &msg)
})
```

Failed sends are automatically retried by goqite's visibility timeout.

### Email Templating with Maizzle

Use Maizzle (MIT, npm) to build email templates with the same Tailwind
utility classes as the frontend. Maizzle compiles Tailwind CSS into
inlined static HTML at build time — no runtime CSS processing needed.

#### Shared Brand Colors

A single `brand.json` at the project root is the source of truth for
brand colors. Both the frontend Tailwind config and the Maizzle email
Tailwind config import it:

```json
{
  "primary": "#1a1a2e",
  "accent": "#e94560",
  "surface": "#16213e",
  "text": "#eaeaea"
}
```

#### Email Template Structure

```
email/
  templates/
    notification.html    -- Maizzle template using Tailwind classes
  layouts/
    main.html            -- shared layout (header, footer, branding)
  tailwind.config.ts     -- imports brand.json, email-safe config
  config.js              -- Maizzle build config
  dist/                  -- compiled output (gitignored)
```

#### Build Integration

Maizzle builds email templates as a mise task. The compiled HTML output
is embedded into the Go binary via `//go:embed`:

```
email/dist/*.html → go:embed → Go binary
```

At runtime, Go loads the pre-built HTML and injects dynamic values
(recipient, notification ID, request ID) via `html/template`:

```go
//go:embed email/dist/*.html
var emailFS embed.FS

var emailTemplates = template.Must(template.ParseFS(emailFS, "email/dist/*.html"))

func RenderNotification(data NotificationData) (string, string, error) {
    var htmlBuf, textBuf bytes.Buffer
    if err := emailTemplates.ExecuteTemplate(&htmlBuf, "notification.html", data); err != nil {
        return "", "", fmt.Errorf("render html: %w", err)
    }
    if err := emailTemplates.ExecuteTemplate(&textBuf, "notification.txt", data); err != nil {
        return "", "", fmt.Errorf("render text: %w", err)
    }
    return htmlBuf.String(), textBuf.String(), nil
}
```

CSS is already inlined by Maizzle at build time — no go-premailer
needed at runtime. The Go side only handles dynamic value injection.

### Parsing Inbound Email

Use `jhillyerd/enmime` for parsing MIME messages (attachments, HTML,
plaintext). Loads the entire message into memory — suitable for
typical-sized messages.

```go
import "github.com/jhillyerd/enmime"

env, err := enmime.ReadEnvelope(reader)
if err != nil {
    return fmt.Errorf("parse email: %w", err)
}
from := env.GetHeader("From")
subject := env.GetHeader("Subject")
textBody := env.Text
htmlBody := env.HTML
attachments := env.Attachments
```

### Testing Email

Unit tests use a spy implementation of the port interface — no
library needed:

```go
type SpyEmailSender struct {
    Messages []*EmailMessage
    Err      error
}

func (s *SpyEmailSender) Send(ctx context.Context, msg *EmailMessage) error {
    s.Messages = append(s.Messages, msg)
    return s.Err
}
```

For integration tests that need to verify SMTP protocol interaction,
use `mocktools/go-smtp-mock/v2`:

```go
import smtpmock "github.com/mocktools/go-smtp-mock/v2"

srv := smtpmock.New(smtpmock.ConfigurationAttr{})
srv.Start()
defer srv.Stop()

// Point your SMTPSender at srv.PortNumber()
// Send email, then assert:
msgs := srv.Messages()
```

For E2E testing, use Mailpit (standalone SMTP server with web UI and
REST API). It replaces the abandoned MailHog.

### `net/smtp` (stdlib)

The `net/smtp` package is **frozen** — no new features accepted. It
lacks implicit TLS, XOAUTH2 auth, and MIME building. Use `go-mail`
instead.

---

## HTTP Server

### Server Setup

Use an exported `RunServer(ctx, cfg)` function that blocks until
shutdown. This lets tests call it with a cancellable context.

```go
type ServerConfig struct {
    Bind            string
    Port            int
    ShutdownTimeout time.Duration
    // ... other fields (DSN, SMTP, etc.)
}

// RunServer starts the HTTP server and blocks until ctx is cancelled
// or a fatal error occurs. Exported so tests can call it directly.
func RunServer(ctx context.Context, cfg ServerConfig) error {
    store, err := db.Open(cfg.DSN)
    if err != nil {
        return fmt.Errorf("open store: %w", err)
    }
    defer func() { _ = store.Close() }()

    mux := http.NewServeMux()
    mux.HandleFunc("GET /v1/health", httpapi.HandleHealth(store))

    mcpHandler := mcp.NewMCPHandler(store, enqueuer, cfg.Version)
    mux.Handle("/mcp/", http.StripPrefix("/mcp", mcpHandler))

    handler := httpapi.WithMiddleware(mux)

    srv := &http.Server{
        Addr:              fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port),
        Handler:           handler,
        ReadTimeout:       15 * time.Second,
        WriteTimeout:      15 * time.Second,
        IdleTimeout:       60 * time.Second,
        ReadHeaderTimeout: 5 * time.Second,
    }
    // ... start server and handle shutdown (see Daemon Subcommand)
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

Three layers, applied outermost-first: request ID, request logging,
panic recovery. All exported for testing and reuse.

```go
type contextKey string

const requestIDKey contextKey = "request_id"

// WithRequestID generates a unique request ID, stores it in context,
// and sets the X-Request-ID response header. Outermost layer so all
// downstream middleware and handlers can read it.
func WithRequestID(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        reqID := domain.NewID("req")
        w.Header().Set("X-Request-ID", reqID)
        ctx := context.WithValue(r.Context(), requestIDKey, reqID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func RequestIDFromContext(ctx context.Context) string {
    if v, ok := ctx.Value(requestIDKey).(string); ok {
        return v
    }
    return ""
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

func WithRequestLogging(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
        next.ServeHTTP(rec, r)
        slog.Info("request",
            "method", r.Method,
            "path", r.URL.Path,
            "status", rec.status,
            "duration", time.Since(start),
            "request_id", RequestIDFromContext(r.Context()),
        )
    })
}

func WithPanicRecovery(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if err := recover(); err != nil {
                slog.Error("panic recovered",
                    "error", err,
                    "method", r.Method,
                    "path", r.URL.Path,
                )
                if rec, ok := w.(*statusRecorder); ok && !rec.written {
                    http.Error(w, "internal server error", http.StatusInternalServerError)
                }
            }
        }()
        next.ServeHTTP(w, r)
    })
}

// WithMiddleware applies the full stack: RequestID → Logging → PanicRecovery.
func WithMiddleware(next http.Handler) http.Handler {
    return WithRequestID(WithRequestLogging(WithPanicRecovery(next)))
}
```

---

## Authentication

Authentication is intentionally out of scope for this architecture
reference. Most organizations have an existing standard — SSO, JWT,
OAuth2, API keys, or a combination — and the choice depends on
infrastructure, compliance requirements, and team preferences.

The architecture supports any approach through the same patterns used
for other cross-cutting concerns:

- **Middleware** extracts and validates credentials (token, cookie,
  API key) from the request. This is an infra concern.
- **Context propagation** carries the authenticated identity from
  middleware into handlers and services. Domain code accesses the
  identity via context, never by inspecting headers directly.
- **Port interfaces** define an `IdentityProvider` or `AuthProvider`
  in the domain if services need to make authorization decisions.
  The implementation lives in infra.

```go
// Domain port — does not know how auth works, only what it provides.
type IdentityProvider interface {
    Identify(ctx context.Context) (*Identity, error)
}

type Identity struct {
    UserID string
    Roles  []string
}
```

The middleware, provider implementation, and token validation logic
are left to the implementer. Common options include `lestrrat-go/jwx`
for JWT, `coreos/go-oidc` for OpenID Connect, or a custom middleware
calling an internal auth service.

## Hardening

Security concerns that are out of scope for this architecture but
should be addressed before production deployment:

**Request Body Size Limits.** Wrap request bodies with
`http.MaxBytesReader` to prevent memory exhaustion from unbounded
POST bodies:

```go
r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
```

**CORS.** Configure `Access-Control-Allow-Origin` to restrict which
origins can call the API. Without CORS headers, the API cannot
distinguish same-origin from cross-origin requests, which is relevant
for CSRF. Use middleware to set allowed origins, methods, and headers.

**Rate Limiting.** Add per-IP or per-token rate limiting to protect
against abuse. Endpoints that create records, enqueue jobs, or send
email are especially sensitive. Options include `golang.org/x/time/rate`
for in-process limiting or a reverse proxy (nginx, Caddy) for
infrastructure-level limiting.

**WebSocket Origin Validation.** Implemented in `HandleWS` via
`websocket.AcceptOptions{OriginPatterns}` (see WebSocket section).

**WebSocket Read Limits.** Implemented in `ReadPump` via
`SetReadLimit(512)` (see WebSocket section).

**WebSocket Connection Limits.** Implemented in `NewHub(maxConns)`
(see WebSocket section).

**Error Response Sanitization.** Implemented in MCP tool handlers
(see MCP section). Log real errors server-side, return generic
messages to clients.

---

## MCP Integration

### Server Side

Return a `*server.StreamableHTTPServer` so the caller can call
`Shutdown` during graceful shutdown:

```go
func NewMCPHandler(store MCPStore, enqueuer domain.Enqueuer, version string) *server.StreamableHTTPServer {
    s := server.NewMCPServer("myservice", version)
    s.AddTool(mcp.NewTool("send_notification",
        mcp.WithDescription("Send a notification email"),
        mcp.WithString("email", mcp.Required(), mcp.Description("Email address")),
    ), HandleSendNotification(store, enqueuer))
    return server.NewStreamableHTTPServer(s)
}
```

Mount with `http.StripPrefix`:

```go
mux.Handle("/mcp/", http.StripPrefix("/mcp", mcpHandler))
```

### Error Sanitization

MCP tool handlers log the real error server-side and return a generic
message to the client. Never expose `err.Error()` to MCP clients.

```go
if err := store.CreateNotification(ctx, n); err != nil {
    if errors.Is(err, domain.ErrAlreadyNotified) {
        return mcp.NewToolResultError("already notified"), nil
    }
    slog.Error("create notification failed", "error", err)
    return mcp.NewToolResultError("internal error"), nil
}
```

### Bridge Subcommand

`cmd/mcpbridge/` — reads JSON-RPC from stdin, forwards to
`http://<host>:<port>/mcp`, relays responses to stdout. Passes auth
token on every request.

---

## WebSocket

Real-time push to connected clients using
[coder/websocket](https://github.com/coder/websocket) (ISC license,
zero external deps). Import: `github.com/coder/websocket`.

### Hub Pattern

A hub manages connected clients and broadcasts messages. Standard
pattern for real-time dashboards. No mutex needed — the single-goroutine
`Run` loop is the sole writer to the `clients` map; all external access
goes through channels.

```go
type Hub struct {
    clients    map[*Client]struct{}
    broadcast  chan []byte
    register   chan *Client
    unregister chan *Client
    maxConns   int
}

type Client struct {
    hub  *Hub
    Conn *websocket.Conn
    send chan []byte
}

// NewHub creates a hub. maxConns caps concurrent connections;
// registrations above this limit are rejected.
func NewHub(maxConns int) *Hub {
    return &Hub{
        clients:    make(map[*Client]struct{}),
        broadcast:  make(chan []byte, 256),
        register:   make(chan *Client),
        unregister: make(chan *Client),
        maxConns:   maxConns,
    }
}

// Register adds a client via the channel (goroutine-safe).
func (h *Hub) Register(c *Client) { h.register <- c }

// Broadcast sends a message to all clients via the channel.
func (h *Hub) Broadcast(msg []byte) { h.broadcast <- msg }

func (h *Hub) Run(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case c := <-h.register:
            if len(h.clients) >= h.maxConns {
                close(c.send) // reject — WritePump detects and closes conn
            } else {
                h.clients[c] = struct{}{}
            }
        case c := <-h.unregister:
            if _, ok := h.clients[c]; ok {
                delete(h.clients, c)
                close(c.send)
            }
        case msg := <-h.broadcast:
            for c := range h.clients {
                select {
                case c.send <- msg:
                default:
                    // Slow client — drop it.
                    delete(h.clients, c)
                    close(c.send)
                }
            }
        }
    }
}
```

### Client Read/Write Pumps

Exported so handler code can call them directly.

```go
func (c *Client) WritePump(ctx context.Context) {
    defer func() { _ = c.Conn.Close(websocket.StatusNormalClosure, "closing") }()
    for {
        select {
        case <-ctx.Done():
            return
        case msg, ok := <-c.send:
            if !ok {
                return
            }
            if err := c.Conn.Write(ctx, websocket.MessageText, msg); err != nil {
                return
            }
        }
    }
}

func (c *Client) ReadPump(ctx context.Context) {
    defer func() { c.hub.unregister <- c }()
    c.Conn.SetReadLimit(512) // prevent memory exhaustion from large frames
    for {
        if _, _, err := c.Conn.Read(ctx); err != nil {
            return
        }
    }
}
```

### HTTP Upgrade Handler

The `connCtx` parameter provides the lifecycle context for connections.
After `websocket.Accept` hijacks the connection, `r.Context()` is
unreliable (may be cancelled when the handler returns). Use a context
derived from the hub's lifecycle instead.

Origin validation is enforced via `websocket.AcceptOptions{OriginPatterns}`.

```go
func HandleWS(hub *Hub, connCtx context.Context, allowedOrigins []string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
            OriginPatterns: allowedOrigins,
        })
        if err != nil {
            return // Accept writes the HTTP error
        }
        client := NewClient(hub, conn)
        hub.Register(client)

        go client.WritePump(connCtx)
        client.ReadPump(connCtx) // blocks until disconnect
    }
}
```

### Integration

Mount on the server mux alongside REST routes:

```go
mux.HandleFunc("GET /v1/ws", ws.HandleWS(hub, hubCtx, allowedOrigins))
```

Start the hub in the daemon before the HTTP server, with a separate
context so write pumps can still dispatch messages during graceful
shutdown:

```go
hubCtx, hubCancel := context.WithCancel(context.Background())
defer hubCancel()

hub := ws.NewHub(1000) // max concurrent connections
go hub.Run(hubCtx)
```

Broadcast state changes from the service/store layer:

```go
func (s *Service) UpdateStatus(ctx context.Context, id string, status Status) error {
    if err := s.store.UpdateStatus(ctx, id, status); err != nil {
        return err
    }
    msg, _ := json.Marshal(map[string]string{"id": id, "status": string(status)})
    s.hub.Broadcast(msg)
    return nil
}
```

---

## Health Endpoint

Accept the narrow `domain.HealthChecker` interface, not the full `Store`:

```go
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
```

---

## Daemon Subcommand

The `RunE` parses flags and delegates to `RunServer`, which is exported
so tests can call it with a cancellable context.

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
    cmd.Flags().Int("shutdown-timeout", 60, "Shutdown timeout in seconds")
    return cmd
}

func run(cmd *cobra.Command, args []string) error {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()
    return RunServer(ctx, ServerConfig{...})
}
```

### Separate Contexts per Subsystem

Each long-running subsystem (hub, job runner) gets its own context
derived from `context.Background()` — not from the signal context.
This allows ordered shutdown: the signal context triggers shutdown, but
subsystem contexts are cancelled individually in the correct sequence.

```go
hubCtx, hubCancel := context.WithCancel(context.Background())
defer hubCancel()
go hub.Run(hubCtx)

runnerCtx, runnerCancel := context.WithCancel(context.Background())
defer runnerCancel()
runnerDone := make(chan struct{})
go func() {
    runner.Start(runnerCtx)
    close(runnerDone)
}()
```

### Ordered Graceful Shutdown

A 5-step sequence ensures no messages are lost and no database calls
happen after the store is closed. The shutdown timeout is configurable
(default 60s).

```go
shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
defer shutdownCancel()

// 1. Drain HTTP — reject new requests, let in-flight handlers finish.
if err := srv.Shutdown(shutdownCtx); err != nil {
    slog.Error("http shutdown", "error", err)
}

// 2. Signal MCP SSE clients to reconnect.
if err := mcpHandler.Shutdown(shutdownCtx); err != nil {
    slog.Error("mcp shutdown", "error", err)
}

// 3. Cancel hub — write pumps close WS connections with StatusNormalClosure.
hubCancel()

// 4. Cancel runner — job runner drains current job, then exits.
runnerCancel()

// 5. Wait for in-flight jobs, bounded by shutdown timeout.
select {
case <-runnerDone:
    slog.Info("runner stopped")
case <-shutdownCtx.Done():
    slog.Warn("shutdown timeout waiting for runner")
}

// 6. Store close happens via defer.
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
- E2E tests in a separate `tests/e2e/` directory with its own `go.mod`

### Test Fixture Return Types

Test helpers **must return port interfaces, never concrete adapter types.** Even when a helper constructs a specific adapter internally (e.g., `sqlite.Open(":memory:")`), the return type must be the port:

```go
// WRONG — leaks the adapter, makes every test SQLite-only by inference
func newTestStore(t *testing.T) *sqlite.Store {
    s, err := sqlite.Open(":memory:")
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { s.Close() })
    return s
}

// RIGHT — internal construction, port-typed return
func newTestStore(t *testing.T) domain.Store {
    s, err := sqlite.Open(":memory:")
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { s.Close() })
    return s
}
```

Why each part of this rule matters:

- **The return type signals contract scope.** Returning `domain.Store` tells readers (and grep/lint tools) that the test exercises the port contract, not adapter-specific behavior. Returning `*sqlite.Store` silently narrows every caller to one backend.
- **Future adapters plug in without test changes.** A second store implementation can be wired in without touching any test that already uses the port-typed helper.
- **Adapter-specific behavior stays obviously isolated.** Code that legitimately needs a concrete type (e.g. calling `.SchemaVersion()` only on Postgres) should be in an explicitly named helper — not the silent default.

When a test legitimately needs an adapter-specific method, factor a separate helper with an explicit name:

```go
func newSQLiteSchemaProbe(t *testing.T) *sqlite.Store {
    s, err := sqlite.Open(":memory:")
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { s.Close() })
    return s
}
```

The concrete return type on a dedicated helper is a visible signal that the test is probing adapter internals — intentional, not accidental.

Real-world note: this violation is easy to introduce silently and hard to notice in review. A service where production code is hexagonally clean can still have every unit test implicitly SQLite-only if all its `newTestStore` helpers return `*sqlite.Store`. The port-typed convention is the only way to make backend portability testable by default, not by inspection.

### End-to-End Tests

E2E tests live in `tests/e2e/` with a separate `go.mod` so they can
import the project's client SDK without circular dependencies. They
build the real binary, start it as a subprocess, and make HTTP requests
against it.

#### Directory Structure

```
tests/
  e2e/
    go.mod              -- separate module, imports main project
    main_test.go        -- TestMain: build binary, shared setup
    harness_test.go     -- or harness/ package: daemon lifecycle
    health_test.go      -- test files by feature
    entities_test.go
```

#### TestMain — Build Once, Share Across Tests

```go
var serviceBin string

func TestMain(m *testing.M) {
    tmp, err := os.MkdirTemp("", "e2e-*")
    if err != nil {
        log.Fatal(err)
    }
    defer os.RemoveAll(tmp)

    binPath := filepath.Join(tmp, "myservice")
    cmd := exec.Command("go", "build", "-race", "-o", binPath, ".")
    cmd.Dir = filepath.Join("..", "..")
    if out, err := cmd.CombinedOutput(); err != nil {
        log.Fatalf("build failed: %s\n%s", err, out)
    }
    serviceBin = binPath

    os.Exit(m.Run())
}
```

The `-race` flag enables the race detector. The harness checks stderr
for "DATA RACE" on shutdown — a race in any test fails the suite.

#### Free Port Discovery

```go
func FreePort() (string, error) {
    ln, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil {
        return "", err
    }
    addr := ln.Addr().String()
    ln.Close()
    return addr, nil
}
```

#### Daemon Harness

The harness starts the binary, waits for it to accept connections, and
stops it cleanly on test cleanup.

```go
type Daemon struct {
    cmd    *exec.Cmd
    addr   string
    dir    string
    stderr *bytes.Buffer
}

func StartDaemon(bin, addr string, opts ...DaemonOption) (*Daemon, error) {
    cfg := defaultConfig()
    for _, opt := range opts {
        opt(&cfg)
    }

    dir, _ := os.MkdirTemp("", "e2e-daemon-*")

    d := &Daemon{
        addr:   addr,
        dir:    dir,
        stderr: &bytes.Buffer{},
    }

    d.cmd = exec.Command(bin, "daemon", "--bind", "127.0.0.1", "--port", port(addr))
    d.cmd.Stderr = d.stderr
    d.cmd.Env = append(os.Environ(),
        "XDG_CONFIG_HOME="+filepath.Join(dir, "config"),
        "XDG_STATE_HOME="+filepath.Join(dir, "state"),
    )

    if err := d.cmd.Start(); err != nil {
        return nil, fmt.Errorf("start daemon: %w", err)
    }

    // Poll until the daemon accepts TCP connections
    deadline := time.Now().Add(5 * time.Second)
    for time.Now().Before(deadline) {
        conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
        if err == nil {
            conn.Close()
            return d, nil
        }
        time.Sleep(50 * time.Millisecond)
    }
    d.Stop()
    return nil, fmt.Errorf("daemon did not become ready: %s", d.stderr.String())
}
```

#### Functional Options for Daemon Config

```go
type daemonConfig struct {
    dbPath string
    // add fields as the service grows
}

func defaultConfig() daemonConfig {
    return daemonConfig{dbPath: ""}
}

type DaemonOption func(*daemonConfig)

func WithDB(path string) DaemonOption {
    return func(c *daemonConfig) { c.dbPath = path }
}
```

#### Stop with Race Detection

```go
func (d *Daemon) Stop() error {
    if d.cmd.Process != nil {
        d.cmd.Process.Signal(syscall.SIGTERM)
        done := make(chan error, 1)
        go func() { done <- d.cmd.Wait() }()
        select {
        case <-time.After(5 * time.Second):
            d.cmd.Process.Kill()
        case <-done:
        }
    }
    os.RemoveAll(d.dir)
    return nil
}

func (d *Daemon) StopFatal(t *testing.T) {
    t.Helper()
    d.Stop()
    if strings.Contains(d.stderr.String(), "DATA RACE") {
        t.Fatalf("data race detected:\n%s", d.stderr.String())
    }
}
```

#### Orphan-Process Hardening (Required)

The minimal `StartDaemon`/`Stop` shape above leaks orphan processes
when the daemon spawns descendants (containerd shims, helper binaries,
forked workers) or buffers stderr through an `io.Writer`. The leaked
descendants keep stderr pipes alive, so `cmd.Wait()` blocks forever
and the test step hangs until the CI workflow timeout cancels it.
A real incident: Sharkfin's CI step hung for ~55 minutes after a
single test failure left an orphan child holding the stderr pipe.

The canonical fix has four parts. **Each is load-bearing — drop any
one and the leak returns.**

```go
import "syscall"

// In StartDaemon:
stderrFile, err := os.CreateTemp("", "<svc>-e2e-stderr-*")
if err != nil {
    // cleanup, return
}
cmd := exec.Command(binary, args...)
cmd.Env = append(os.Environ(), /* ... */)
cmd.Stdout = stderrFile
cmd.Stderr = stderrFile
cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
cmd.WaitDelay = 10 * time.Second // Go 1.20+; force-close I/O after exit
if err := cmd.Start(); err != nil {
    stderrFile.Close()
    os.Remove(stderrFile.Name())
    // cleanup
    return nil, err
}

// In Stop:
pgid := cmd.Process.Pid // pgid == pid because of Setpgid
_ = syscall.Kill(-pgid, syscall.SIGTERM)
done := make(chan error, 1)
go func() { done <- cmd.Wait() }()
select {
case <-done:
case <-time.After(5 * time.Second):
    _ = syscall.Kill(-pgid, syscall.SIGKILL)
    <-done
}
```

Why each part is load-bearing:

1. **`Setpgid: true`** — places the daemon and every descendant in a
   fresh process group whose pgid equals the daemon PID. Without it,
   `kill -PID` only signals the daemon; orphans inherit init.
2. **`*os.File` for stdout/stderr (NOT `io.Writer`)** — when stderr
   is an `io.Writer` (e.g. `bytes.Buffer`, `io.MultiWriter`),
   `exec.Cmd` creates an OS pipe and a goroutine that copies bytes
   off the read end. A descendant that inherits the write end keeps
   the pipe open after the daemon exits, so the copy goroutine never
   sees EOF and `cmd.Wait()` blocks forever. Writing directly to an
   `*os.File` skips the pipe entirely.
3. **Negative-pid kill (`syscall.Kill(-pgid, sig)`)** — signals the
   whole process group, killing leaked descendants alongside the
   daemon. `cmd.Process.Signal` only signals the daemon itself.
4. **`cmd.WaitDelay = 10 * time.Second`** — Go 1.20+ safety net.
   If a descendant still holds an inherited fd after the daemon
   process exits, `WaitDelay` makes `cmd.Wait()` close the I/O
   sources and return after the delay instead of blocking forever.

The pattern is Linux/Unix-only: `syscall.SysProcAttr.Setpgid` does
not exist on Windows. All WorkFort e2e suites run on Linux today.

To verify the fix in a test, signal the group with `sig 0` after
`Stop` returns: `syscall.Kill(-pgid, 0)` returns
`syscall.ESRCH` when the group is empty (no remaining members).
This is the canonical "is the group empty?" probe and is the
recommended assertion for leak-detection tests:

```go
if err := syscall.Kill(-pgid, 0); !errors.Is(err, syscall.ESRCH) {
    t.Fatalf("kill(-%d, 0) = %v, want ESRCH", pgid, err)
}
```

To capture stderr for log-dumps or DATA RACE detection, read the
`*os.File` after `cmd.Wait()` returns:

```go
if t.Failed() {
    data, _ := os.ReadFile(stderrFile.Name())
    t.Logf("daemon stderr:\n%s", data)
}
data, _ := os.ReadFile(stderrFile.Name())
if bytes.Contains(data, []byte("DATA RACE")) {
    t.Fatal("data race detected in daemon")
}
stderrFile.Close()
os.Remove(stderrFile.Name())
```

Apply the same four-part pattern to every long-lived subprocess the
harness spawns — daemons, MCP bridges, workers, anything that itself
forks. The pattern is mandatory for new harnesses and required when
auditing existing ones for CI hangs.

##### Test-runner wiring (also required)

The four-part code fix only matters if the test runner actually
invokes the leak test. Two systematic gaps surfaced during the
2026-04-19 cross-repo rollout — affected pylon, nexus, and sharkfin:

1. **Binary env var must be exported by the runner.** Leak tests
   typically gate on a `<SVC>_BINARY` env var (and `t.Skip` when
   unset, so unit-test runs without a built binary don't fail).
   The `mise run e2e` task (or equivalent) must export this env
   var before invoking `go test`. Without it the leak test
   silently SKIPs in CI — the regression guard is effectively dead:

   ```bash
   #!/usr/bin/env bash
   #MISE depends=["build:dev"]
   set -euo pipefail
   export <SVC>_BINARY="${MISE_PROJECT_ROOT}/build/<svc>"
   cd tests/e2e && go test -v -race -timeout 180s ./...
   ```

2. **`go test` glob must reach the leak test's package.** Bare
   `go test` (with no path arg) and `go test .` only run the
   current package. If the leak test lives in a subpackage
   (e.g. `tests/e2e/harness/`), use `./...` to include it:

   ```
   go test ... ./...   # includes subpackages — correct
   go test ... .       # current package only — leak test never reached
   go test ...         # same as above — current package only
   ```

When applying this orphan-hardening pattern to a repo, audit the
mise/CI test runner alongside the harness code. After the fix
lands, verify by running the runner once and checking the leak
test's output is `--- PASS` (not `--- SKIP`).

#### Multi-Daemon Test Isolation (Per-Backend)

E2E tests that need multiple daemon processes **must be able to give each daemon an isolated backend instance, regardless of which backend the suite is running against.** SQLite isolation = a separate tempfile per daemon. Postgres isolation = a separate sibling database per daemon (e.g. `<svc>_test_b`, `<svc>_test_c`), each with its schema reset before the test and dropped after.

The harness exposes this via a `FreshDB` primitive:

```go
// FreshDB returns a DSN for a sibling database that is isolated from the
// default test database. Each call provisions one sibling.
//
// SQLite: returns a new tempfile path; the daemon opens it on startup.
// Postgres: creates (or reuses) <svc>_test_b (cycling _c, _d, ...)
//   and truncates the public schema. Cleanup drops the schema.
func (h *Harness) FreshDB(t *testing.T) string {
    // backend-detect, provision sibling, reset schema, return DSN
}
```

Tests then express two-daemon isolation without knowing which backend is active:

```go
addrA, _ := harness.FreePort()
dA, _ := harness.StartDaemon(svcBin, addrA)        // default DB
defer dA.Cleanup()

dsnB := harness.FreshDB(t)                          // sibling DB
addrB, _ := harness.FreePort()
dB, _ := harness.StartDaemon(svcBin, addrB, harness.WithDB(dsnB))
defer dB.Cleanup()
```

**Anti-pattern — what not to do:**

```go
if os.Getenv("MYSERVICE_DB") != "" {
    t.Skip("this test requires SQLite (MYSERVICE_DB must not be set)")
}
```

This is silently negative-coverage: the test is in the suite but doesn't run on the production-relevant backend. If the harness lacks the primitive to support a test's shape, the right move is to **add the primitive** (i.e. implement `FreshDB`).

If a skip is genuinely unavoidable today, the documentation rule applies: **every conditional `t.Skip` in an e2e test must be cross-referenced from `docs/remaining-work.md`** with a one-line note covering which test, which condition, and what work removes the skip. A skip with no paper trail is indistinguishable from an accidental omission — and will be treated as one six weeks later.

Why this matters: when a service supports dual backends, an e2e test that can only run on SQLite is not testing the production path. The skip doesn't surface in CI output as a failure; it just disappears. The only way to catch that a feature works on Postgres is to run the test on Postgres — which requires the harness to support it.

#### Writing Tests

```go
func TestHealth(t *testing.T) {
    addr, _ := FreePort()
    d, err := StartDaemon(serviceBin, addr)
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { d.StopFatal(t) })

    resp, err := http.Get(fmt.Sprintf("http://%s/v1/health", addr))
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        t.Fatalf("expected 200, got %d", resp.StatusCode)
    }

    var body map[string]string
    json.NewDecoder(resp.Body).Decode(&body)
    if body["status"] != "healthy" {
        t.Fatalf("expected healthy, got %s", body["status"])
    }
}

func TestEntityCRUD(t *testing.T) {
    addr, _ := FreePort()
    d, _ := StartDaemon(serviceBin, addr)
    t.Cleanup(func() { d.StopFatal(t) })

    base := fmt.Sprintf("http://%s/v1", addr)

    // Create
    body := `{"name":"test"}`
    resp, _ := http.Post(base+"/entities", "application/json", strings.NewReader(body))
    if resp.StatusCode != http.StatusCreated {
        t.Fatalf("create: expected 201, got %d", resp.StatusCode)
    }
    var created Entity
    json.NewDecoder(resp.Body).Decode(&created)
    resp.Body.Close()

    // Get
    resp, _ = http.Get(fmt.Sprintf("%s/entities/%s", base, created.ID))
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("get: expected 200, got %d", resp.StatusCode)
    }
    resp.Body.Close()

    // Delete
    req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/entities/%s", base, created.ID), nil)
    resp, _ = http.DefaultClient.Do(req)
    if resp.StatusCode != http.StatusNoContent {
        t.Fatalf("delete: expected 204, got %d", resp.StatusCode)
    }
    resp.Body.Close()
}
```

Each test gets its own daemon with an isolated in-memory database.
Tests can run in parallel with `t.Parallel()` since each daemon binds
to a different port.

#### Spec-Driven E2E Tests

When OpenSpec specs exist (`openspec/specs/`), use their Given-When-Then
scenarios to structure e2e tests. Each scenario maps to a test case:

```go
// Spec: auth-session, Scenario: Default session timeout
// GIVEN a user has authenticated
// WHEN 24 hours pass without activity
// THEN invalidate the session token
// AND require re-authentication
func TestSessionTimeout(t *testing.T) {
    addr, _ := FreePort()
    d, _ := StartDaemon(serviceBin, addr)
    t.Cleanup(func() { d.StopFatal(t) })

    base := fmt.Sprintf("http://%s/v1", addr)

    // GIVEN: authenticate
    token := authenticate(t, base, "user@example.com", "password")

    // WHEN: session expires (use short timeout in test config)
    time.Sleep(testSessionTimeout + 100*time.Millisecond)

    // THEN: token is rejected
    req, _ := http.NewRequest("GET", base+"/me", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    resp, _ := http.DefaultClient.Do(req)
    if resp.StatusCode != http.StatusUnauthorized {
        t.Fatalf("expected 401 after timeout, got %d", resp.StatusCode)
    }
    resp.Body.Close()
}
```

Name tests after the spec and scenario they verify. This makes it easy
to trace test failures back to spec requirements and keeps QA aligned
with the spec-driven workflow.

---

## Build Tooling

### mise.toml

The `mise.toml` at the repo root pins tool versions only. Tasks live
in separate files under `.mise/tasks/`.

```toml
[tools]
go = "1.26.0"
```

### Task Files

Each task is an executable bash script in `.mise/tasks/`. Subdirectories
create colon-separated namespaces: `.mise/tasks/build/go` runs as
`mise run build:go`.

```
.mise/
  tasks/
    build/
      go              -- mise run build:go
    test/
      unit            -- mise run test:unit
      e2e             -- mise run test:e2e
    release/
      dev             -- mise run release:dev
      production      -- mise run release:production
    lint/
      go              -- mise run lint:go
    clean/
      go              -- mise run clean:go
    ci                -- mise run ci
```

**`.mise/tasks/build/go`:**

```bash
#!/usr/bin/env bash
#MISE description="Build the binary (debug)"
set -euo pipefail

go build -o build/myservice .
```

**`.mise/tasks/test/unit`:**

```bash
#!/usr/bin/env bash
#MISE description="Run unit tests"
set -euo pipefail

go test ./...
```

**`.mise/tasks/test/e2e`:**

```bash
#!/usr/bin/env bash
#MISE description="Run end-to-end tests"
#MISE depends=["build:go"]
set -euo pipefail

cd tests/e2e
go test -v -count=1 ./...
```

**`.mise/tasks/release/dev`:**

```bash
#!/usr/bin/env bash
#MISE description="Build debug binary with race detector"
set -euo pipefail

go build -race -o build/myservice .
```

**`.mise/tasks/release/production`:**

```bash
#!/usr/bin/env bash
#MISE description="Build release binary"
set -euo pipefail

VERSION="${VERSION:-dev}"
CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -trimpath \
    -o build/myservice .
```

**`.mise/tasks/lint/go`:**

```bash
#!/usr/bin/env bash
#MISE description="Run Go linter"
set -euo pipefail

golangci-lint run ./...
```

**`.mise/tasks/clean/go`:**

```bash
#!/usr/bin/env bash
#MISE description="Remove Go build artifacts"
set -euo pipefail

rm -rf build/
```

**`.mise/tasks/ci`:**

```bash
#!/usr/bin/env bash
#MISE description="Run full CI checks"
#MISE depends=["lint:go", "test:unit", "build:go"]
set -euo pipefail

echo "CI passed"
```

Task files must be executable: `chmod +x .mise/tasks/*`

Key metadata directives:
- `#MISE description="..."` — shown in `mise tasks` output
- `#MISE depends=["lint:go", "test:unit"]` — run dependencies first (use colon namespaces)
- `#MISE sources=["pattern"]` — file patterns for cache invalidation

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
