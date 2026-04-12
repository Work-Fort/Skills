package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/workfort/notifier/cmd/daemon"
	mcpbridge "github.com/workfort/notifier/cmd/mcp-bridge"
	"github.com/workfort/notifier/internal/config"
)

// Version is set from main.go, which receives it via -ldflags.
var Version = "dev"

// NewRootCmd creates the root cobra command with PersistentPreRunE
// that initialises XDG directories and loads configuration.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "notifier",
		Short:   "Notification service",
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
	cmd.AddCommand(daemon.NewCmd())
	cmd.AddCommand(mcpbridge.NewCmd())
	return cmd
}

// Execute runs the root command. Exits with code 1 on error.
func Execute() {
	root := NewRootCmd()
	root.Version = Version
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
