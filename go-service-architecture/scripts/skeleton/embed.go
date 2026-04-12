//go:build !spa

package main

import "embed"

// webFS is empty when built without the "spa" tag.
// Use --dev to proxy to Vite's dev server during development.
var webFS embed.FS
