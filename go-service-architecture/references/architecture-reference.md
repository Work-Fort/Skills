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

```go
import gomail "github.com/wneessen/go-mail"

type SMTPSender struct {
    client *gomail.Client
    from   string
}

func NewSMTPSender(host string, port int, user, pass, from string) (*SMTPSender, error) {
    c, err := gomail.NewClient(host,
        gomail.WithPort(port),
        gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
        gomail.WithUsername(user),
        gomail.WithPassword(pass),
        gomail.WithTLSPolicy(gomail.TLSMandatory),
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

### Email Templating

Use `html/template` with embedded templates and `go-premailer` for CSS
inlining (required for email client compatibility):

```go
import (
    "html/template"
    "embed"
    premailer "github.com/vanng822/go-premailer/premailer"
)

//go:embed templates/*.html
var templateFS embed.FS

var emailTemplates = template.Must(template.ParseFS(templateFS, "templates/*.html"))

func RenderEmail(name string, data any) (string, error) {
    var buf bytes.Buffer
    if err := emailTemplates.ExecuteTemplate(&buf, name, data); err != nil {
        return "", fmt.Errorf("render template %s: %w", name, err)
    }
    // Inline CSS for email client compatibility
    p, err := premailer.NewPremailerFromString(buf.String(), nil)
    if err != nil {
        return "", fmt.Errorf("init premailer: %w", err)
    }
    html, err := p.Transform()
    if err != nil {
        return "", fmt.Errorf("inline css: %w", err)
    }
    return html, nil
}
```

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

## WebSocket

Real-time push to connected clients using
[coder/websocket](https://github.com/coder/websocket) (ISC license,
zero external deps). Import: `github.com/coder/websocket`.

### Hub Pattern

A hub manages connected clients and broadcasts messages. Standard
pattern for real-time dashboards.

```go
type Hub struct {
    clients    map[*Client]struct{}
    broadcast  chan []byte
    register   chan *Client
    unregister chan *Client
    mu         sync.Mutex
}

type Client struct {
    hub  *Hub
    conn *websocket.Conn
    send chan []byte
}

func NewHub() *Hub {
    return &Hub{
        clients:    make(map[*Client]struct{}),
        broadcast:  make(chan []byte, 256),
        register:   make(chan *Client),
        unregister: make(chan *Client),
    }
}

func (h *Hub) Run(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case c := <-h.register:
            h.mu.Lock()
            h.clients[c] = struct{}{}
            h.mu.Unlock()
        case c := <-h.unregister:
            h.mu.Lock()
            if _, ok := h.clients[c]; ok {
                delete(h.clients, c)
                close(c.send)
            }
            h.mu.Unlock()
        case msg := <-h.broadcast:
            h.mu.Lock()
            for c := range h.clients {
                select {
                case c.send <- msg:
                default:
                    delete(h.clients, c)
                    close(c.send)
                }
            }
            h.mu.Unlock()
        }
    }
}
```

### Client Read/Write Pumps

```go
func (c *Client) writePump(ctx context.Context) {
    defer c.conn.Close(websocket.StatusNormalClosure, "closing")
    for {
        select {
        case <-ctx.Done():
            return
        case msg, ok := <-c.send:
            if !ok {
                return
            }
            if err := c.conn.Write(ctx, websocket.MessageText, msg); err != nil {
                return
            }
        }
    }
}

func (c *Client) readPump(ctx context.Context) {
    defer func() { c.hub.unregister <- c }()
    for {
        // Read detects disconnects; discard payload if no client messages expected.
        if _, _, err := c.conn.Read(ctx); err != nil {
            return
        }
    }
}
```

### HTTP Upgrade Handler

```go
func handleWS(hub *Hub) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        conn, err := websocket.Accept(w, r, nil)
        if err != nil {
            return // Accept writes the HTTP error
        }
        client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256)}
        hub.register <- client

        ctx := r.Context()
        go client.writePump(ctx)
        client.readPump(ctx) // blocks until disconnect
    }
}
```

### Integration

Mount on the server mux alongside REST routes:

```go
mux.HandleFunc("/v1/ws", handleWS(hub))
```

Start the hub in the daemon's `RunE` before the HTTP server:

```go
hub := NewHub()
go hub.Run(ctx)
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

// Broadcast is a convenience method on Hub.
func (h *Hub) Broadcast(msg []byte) {
    h.broadcast <- msg
}
```

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
- E2E tests in a separate `tests/e2e/` directory with its own `go.mod`

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
