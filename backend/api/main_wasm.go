//go:build js && wasm

package main

import (
	"database/sql"
	"log"
	"strings"
	"time"

	"github.com/mindsgn-studio/afika-backend/api/internal/posservice"
	"github.com/mindsgn-studio/afika-backend/api/internal/posstore"
	"github.com/mindsgn-studio/afika-backend/api/lib"
	"github.com/syumai/workers-go"
	"github.com/syumai/workers-go/cloudflare"
	_ "github.com/syumai/workers-go/cloudflare/d1" // register the "d1" database/sql driver
)

const (
	// defaultDBBinding matches the shared `afika` D1 binding in wrangler.jsonc.
	defaultDBBinding = "afika"
	privyJWKSURL     = "https://auth.privy.io/api/v1/apps/cmuqob0zx011s0cjqgmza7ovk/jwks.json"
)

func main() {
	bindGlobalFetch()
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
	privyNativeAppID := strings.TrimSpace(cloudflare.Getenv("PRIVY_NATIVE_APP_ID"))
	var privyUsers lib.PrivyUserReader
	var passwordless lib.PasswordlessClient
	e2eMode := strings.EqualFold(strings.TrimSpace(cloudflare.Getenv("E2E_MODE")), "true")
	if e2eMode {
		privyUsers = demoUsers{}
		passwordless = demoPasswordless{}
	} else if privyAppID != "" && privyAppSecret != "" {
		privyUsers, err = lib.NewPrivyUserAPI(privyAppID, privyAppSecret)
		if err != nil {
			log.Panicf("[api] configure Privy profile API: %v", err)
		}
	}
	if !e2eMode && privyAppID != "" {
		passwordless = lib.NewPrivyPasswordlessClient(privyAppID, privyClientID, privyNativeAppID)
	}
	sessions := lib.NewSessionCodec(cloudflare.Getenv("SESSION_SECRET"))
	if passwordless != nil && sessions == nil {
		log.Printf("[api] WARNING: SESSION_SECRET is not set; website sign-in is disabled")
	}
	verifiers := lib.AnyTokenVerifier{lib.NewStaticTokenVerifier(apiToken)}
	if !e2eMode && privyAppID != "" {
		privyVerifier, err := lib.NewPrivyTokenVerifier(privyAppID, privyJWKSURL)
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
		POS:              newPOSService(db, binding),
	})
	workers.Serve(handler)
}

// newPOSService wires the merchant POS use cases. Reads use the d1 database/sql
// driver; every multi-row write goes through D1Database.batch() because the D1
// driver has no transaction support.
func newPOSService(db *sql.DB, binding string) *posservice.Service {
	service := posservice.New(posstore.NewWithRunner(db, posstore.NewD1BatchRunner(binding)))
	// Receipts print Cape Town time regardless of where the Worker runs.
	service.SetLocation(time.FixedZone("SAST", 2*3600))
	return service
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
