package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"maragu.dev/goqite"

	"github.com/workfort/notifier/internal/config"
	"github.com/workfort/notifier/internal/infra/db"
	"github.com/workfort/notifier/internal/infra/email"
	"github.com/workfort/notifier/internal/infra/httpapi"
	"github.com/workfort/notifier/internal/infra/queue"
	"github.com/workfort/notifier/internal/infra/seed"
)

// NewCmd creates the daemon subcommand.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the HTTP server",
		RunE:  run,
	}
	cmd.Flags().String("bind", "127.0.0.1", "Bind address")
	cmd.Flags().Int("port", 8080, "Listen port")
	cmd.Flags().String("db", "", "Database DSN (empty = SQLite in XDG state dir)")
	cmd.Flags().String("smtp-host", "127.0.0.1", "SMTP server host")
	cmd.Flags().Int("smtp-port", 1025, "SMTP server port")
	cmd.Flags().String("smtp-from", "notifier@localhost", "Email sender address")
	return cmd
}

func run(cmd *cobra.Command, args []string) error {
	// Initialise structured JSON logging.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	// Resolve flags: koanf (file/env) takes precedence.
	bind := resolveString(cmd, "bind")
	port := resolveInt(cmd, "port")
	dsn := resolveString(cmd, "db")
	smtpHost := resolveString(cmd, "smtp-host")
	smtpPort := resolveInt(cmd, "smtp-port")
	smtpFrom := resolveString(cmd, "smtp-from")

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return RunServer(ctx, ServerConfig{
		Bind:     bind,
		Port:     port,
		DSN:      dsn,
		SMTPHost: smtpHost,
		SMTPPort: smtpPort,
		SMTPFrom: smtpFrom,
	})
}

// ServerConfig holds configuration for RunServer.
type ServerConfig struct {
	Bind     string
	Port     int
	DSN      string
	SMTPHost string
	SMTPPort int
	SMTPFrom string
}

// RunServer starts the HTTP server with the given configuration and
// blocks until the context is cancelled or a fatal error occurs.
// Exported so tests can call it with a cancellable context.
func RunServer(ctx context.Context, cfg ServerConfig) error {
	// Default DSN to XDG state directory.
	if cfg.DSN == "" {
		cfg.DSN = filepath.Join(config.StatePath(), "notifier.db")
	}

	// Open the store (SQLite or PostgreSQL based on DSN).
	store, err := db.Open(cfg.DSN)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = store.Close() }()

	// Determine goqite SQL flavor from DSN.
	var queueFlavor queue.Flavor
	if strings.HasPrefix(cfg.DSN, "postgres") {
		queueFlavor = queue.FlavorPostgres
	}

	// Run QA seed data (no-op in non-QA builds).
	if strings.HasPrefix(cfg.DSN, "postgres") {
		if err := seed.RunSeed(store.DB(), goqite.SQLFlavorPostgreSQL); err != nil {
			return fmt.Errorf("run seed: %w", err)
		}
	} else {
		if err := seed.RunSeed(store.DB()); err != nil {
			return fmt.Errorf("run seed: %w", err)
		}
	}

	// Set up goqite queue and job runner.
	nq, err := queue.NewNotificationQueue(store.DB(), queueFlavor)
	if err != nil {
		return fmt.Errorf("create notification queue: %w", err)
	}

	// Set up SMTP sender.
	sender, err := email.NewSMTPSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPFrom)
	if err != nil {
		return fmt.Errorf("create smtp sender: %w", err)
	}

	// Create and register the email worker.
	worker := queue.NewEmailWorker(store, sender)
	runner := queue.NewJobRunner(nq.Queue())
	runner.Register("send_notification", worker.Handle)

	// Start the job runner in a goroutine.
	go runner.Start(ctx)

	// Build the HTTP mux.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", httpapi.HandleHealth(store))
	mux.HandleFunc("POST /v1/notify", httpapi.HandleNotify(store, nq))
	mux.HandleFunc("POST /v1/notify/reset", httpapi.HandleReset(store))
	mux.HandleFunc("GET /v1/notifications", httpapi.HandleList(store))

	// Apply middleware stack.
	handler := httpapi.WithMiddleware(mux)

	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// resolveString reads from koanf if the key exists (checking both
// hyphenated and dotted forms), otherwise from the cobra flag.
func resolveString(cmd *cobra.Command, key string) string {
	dotKey := strings.ReplaceAll(key, "-", ".")
	if config.K.Exists(dotKey) {
		return config.K.String(dotKey)
	}
	v, _ := cmd.Flags().GetString(key)
	return v
}

// resolveInt reads from koanf if the key exists (checking both
// hyphenated and dotted forms), otherwise from the cobra flag.
func resolveInt(cmd *cobra.Command, key string) int {
	dotKey := strings.ReplaceAll(key, "-", ".")
	if config.K.Exists(dotKey) {
		return config.K.Int(dotKey)
	}
	v, _ := cmd.Flags().GetInt(key)
	return v
}
