package main

import "github.com/workfort/notifier/internal/cli"

// Version is set at build time via -ldflags.
var Version = "dev"

func main() {
	cli.Version = Version
	cli.Execute()
}
