package main

import (
	"fmt"
	"os"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func main() {
	// Cobra CLI will replace this in Step 2. For now, confirm the
	// binary builds and prints its version.
	fmt.Println("notifier", Version)
	os.Exit(0)
}
