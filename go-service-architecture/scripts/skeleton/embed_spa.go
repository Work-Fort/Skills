//go:build spa

package main

import "embed"

// webFS holds the Vite build output. Built via:
//   mise run build:web && go build -tags spa
//
//go:embed all:web/dist
var webFS embed.FS
