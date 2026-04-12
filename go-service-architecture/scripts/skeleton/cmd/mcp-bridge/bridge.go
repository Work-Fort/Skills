package mcpbridge

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/workfort/notifier/internal/config"
)

// NewCmd creates the mcp-bridge subcommand.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp-bridge",
		Short: "Stdio-to-HTTP MCP bridge",
		Long:  "Reads JSON-RPC messages from stdin, forwards them to the MCP HTTP endpoint, and relays responses to stdout.",
		RunE:  runBridge,
	}
	cmd.Flags().String("url", "http://127.0.0.1:8080/mcp", "MCP endpoint URL")
	cmd.Flags().String("token", "", "Auth token for MCP requests")
	return cmd
}

func runBridge(cmd *cobra.Command, _ []string) error {
	url := resolveString(cmd, "url")
	token := resolveString(cmd, "token")
	return Bridge(os.Stdin, os.Stdout, url, token)
}

// Bridge reads newline-delimited JSON-RPC messages from r, POSTs each
// to the given URL, and writes responses to w. Each line of input is
// one JSON-RPC message (REQ-010, REQ-011).
func Bridge(r io.Reader, w io.Writer, url string, token string) error {
	client := &http.Client{}
	scanner := bufio.NewScanner(r)
	// Allow up to 1 MB per line.
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			url,
			bytes.NewReader(line),
		)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		// REQ-012: pass auth token on every request.
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := client.Do(req)
		if err != nil {
			slog.Error("forward request", "error", err)
			continue // skip failed requests, don't crash the bridge
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			slog.Error("read response", "error", err)
			continue
		}

		// Write response to stdout, one line per response.
		fmt.Fprintf(w, "%s\n", bytes.TrimSpace(body))
	}

	return scanner.Err()
}

// resolveString reads from koanf if the key exists, otherwise from
// the cobra flag.
func resolveString(cmd *cobra.Command, key string) string {
	dotKey := strings.ReplaceAll(key, "-", ".")
	if config.K.Exists(dotKey) {
		return config.K.String(dotKey)
	}
	v, _ := cmd.Flags().GetString(key)
	return v
}
