//go:build js && wasm

package main

import (
	"database/sql"
	"log"
	"strings"

	"github.com/mindsgn-studio/afika-backend/api/lib"
	"github.com/syumai/workers-go"
	"github.com/syumai/workers-go/cloudflare"
	_ "github.com/syumai/workers-go/cloudflare/d1" // register the "d1" database/sql driver
)

const (
	// defaultDBBinding matches the shared `afika` D1 binding in wrangler.jsonc.
	defaultDBBinding = "afika"
)

func main() {
	binding := envOrDefault("DB_BINDING", defaultDBBinding)
	db, err := sql.Open("d1", binding)
	if err != nil {
		log.Panicf("[api] open D1 binding %q: %v", binding, err)
	}
	defer db.Close()

	apiToken := strings.TrimSpace(cloudflare.Getenv("API_TOKEN"))
	if apiToken == "" {
		log.Printf("[api] WARNING: API_TOKEN is not set; every authenticated request will be rejected")
	}

	handler := lib.NewHandler(lib.HandlerConfig{
		Store:            lib.NewD1Store(db),
		Verifier:         lib.NewStaticTokenVerifier(apiToken),
		AllowedOrigins:   splitList(cloudflare.Getenv("API_ALLOWED_ORIGINS")),
		ReceivingAddress: strings.TrimSpace(cloudflare.Getenv("POS_RECEIVING_ADDRESS")),
		CheckoutBaseURL:  strings.TrimSpace(cloudflare.Getenv("CHECKOUT_BASE_URL")),
	})
	workers.Serve(handler)
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(cloudflare.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func splitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
