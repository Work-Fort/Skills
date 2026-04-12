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
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/workfort/notifier/internal/config"
	"github.com/workfort/notifier/internal/infra/httpapi"
	"github.com/workfort/notifier/internal/infra/seed"
	"github.com/workfort/notifier/internal/infra/sqlite"
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
	return cmd
}

func run(cmd *cobra.Command, args []string) error {
	// Initialise structured JSON logging.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	// Resolve flags: koanf (file/env) takes precedence if the key
	// exists; otherwise fall back to the CLI flag default. Using
	// K.Exists() instead of checking for zero values ensures that
	// intentional zero/empty values (e.g., port 0 for random port,
	// empty bind for all interfaces) are honoured.
	var bind string
	if config.K.Exists("bind") {
		bind = config.K.String("bind")
	} else {
		bind, _ = cmd.Flags().GetString("bind")
	}
	var port int
	if config.K.Exists("port") {
		port = config.K.Int("port")
	} else {
		port, _ = cmd.Flags().GetInt("port")
	}
	var dsn string
	if config.K.Exists("db") {
		dsn = config.K.String("db")
	} else {
		dsn, _ = cmd.Flags().GetString("db")
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return RunServer(ctx, bind, port, dsn)
}

// RunServer starts the HTTP server with the given configuration and
// blocks until the context is cancelled or a fatal error occurs.
// Exported so tests can call it with a cancellable context instead of
// relying on process-wide signal delivery.
func RunServer(ctx context.Context, bind string, port int, dsn string) error {
	// If DSN is still empty (no config/env/flag override), use SQLite
	// in the XDG state directory.
	if dsn == "" {
		dsn = filepath.Join(config.StatePath(), "notifier.db")
	}

	// Open the store.
	store, err := sqlite.Open(dsn)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = store.Close() }()

	// Run QA seed data (no-op in non-QA builds).
	if err := seed.RunSeed(store.DB()); err != nil {
		return fmt.Errorf("run seed: %w", err)
	}

	// Build the HTTP mux.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", httpapi.HandleHealth(store))

	// Apply middleware stack.
	handler := httpapi.WithMiddleware(mux)

	addr := fmt.Sprintf("%s:%d", bind, port)
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
