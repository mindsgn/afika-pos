//go:build !js

package main

import (
	"fmt"
	"os"
)

// The worker targets Cloudflare Workers (GOOS=js GOARCH=wasm). This stub keeps
// native builds and `go test ./...` working.
func main() {
	fmt.Fprintln(os.Stderr, "worker-transaction targets Cloudflare Workers; build with GOOS=js GOARCH=wasm")
	os.Exit(1)
}
