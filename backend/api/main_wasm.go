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
	privyAppID := strings.TrimSpace(cloudflare.Getenv("PRIVY_APP_ID"))
	privyAppSecret := strings.TrimSpace(cloudflare.Getenv("PRIVY_APP_SECRET"))
	privyClientID := strings.TrimSpace(cloudflare.Getenv("PRIVY_CLIENT_ID"))
	var privyUsers lib.PrivyUserReader
	if privyAppID != "" && privyAppSecret != "" {
		privyUsers, err = lib.NewPrivyUserAPI(privyAppID, privyAppSecret)
		if err != nil {
			log.Panicf("[api] configure Privy profile API: %v", err)
		}
	}
	var passwordless lib.PasswordlessClient
	if privyAppID != "" {
		passwordless = lib.NewPrivyPasswordlessClient(privyAppID, privyClientID)
	}
	sessions := lib.NewSessionCodec(cloudflare.Getenv("SESSION_SECRET"))
	if passwordless != nil && sessions == nil {
		log.Printf("[api] WARNING: SESSION_SECRET is not set; website sign-in is disabled")
	}
	verifiers := lib.AnyTokenVerifier{lib.NewStaticTokenVerifier(apiToken)}
	privyKey := strings.TrimSpace(cloudflare.Getenv("PRIVY_VERIFICATION_KEY"))
	if privyAppID != "" || privyKey != "" {
		privyVerifier, err := lib.NewPrivyTokenVerifier(privyAppID, strings.ReplaceAll(privyKey, "\\n", "\n"))
		if err != nil {
			log.Panicf("[api] configure Privy verifier: %v", err)
		}
		verifiers = append(verifiers, privyVerifier)
	}
	if apiToken == "" && len(verifiers) == 1 {
		log.Printf("[api] WARNING: no API_TOKEN or Privy verifier is configured; authenticated requests will be rejected")
	}

	handler := lib.NewHandler(lib.HandlerConfig{
		Store:            lib.NewD1Store(db),
		Verifier:         verifiers,
		PrivyUsers:       privyUsers,
		Passwordless:     passwordless,
		Sessions:         sessions,
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
