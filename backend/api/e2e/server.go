package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/mindsgn-studio/afika-backend/api/lib"
	_ "modernc.org/sqlite"
)

// demoPasswordless stands in for Privy so the browser tests can drive the
// sign-in pages without sending real email. The fixed code matches the one the
// Maestro flows use.
type demoPasswordless struct{}

func (demoPasswordless) SendCode(context.Context, string) error { return nil }

func (demoPasswordless) LoginWithCode(_ context.Context, _ string, code string) (lib.PasswordlessSession, error) {
	if code != "000000" {
		return lib.PasswordlessSession{}, errors.New("invalid code")
	}
	return lib.PasswordlessSession{UserID: "did:privy:e2e-merchant", AccessToken: "e2e-access-token"}, nil
}

// demoUsers returns a stable profile for the signed-in account page.
type demoUsers struct{}

func (demoUsers) GetUser(_ context.Context, userID string) (any, error) {
	return map[string]any{
		"id":         userID,
		"created_at": 1735689600,
		"linked_accounts": []map[string]any{
			{"type": "email", "address": "merchant@example.com"},
		},
	}, nil
}

func main() {
	db, err := sql.Open("sqlite", "file:afika-e2e?mode=memory&cache=shared")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	migration, err := os.ReadFile("migrations/0002_pos_payments.sql")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(string(migration)); err != nil {
		log.Fatal(err)
	}
	h := lib.NewHandler(lib.HandlerConfig{
		Store:                 lib.NewD1Store(db),
		Verifier:              lib.NewStaticTokenVerifier("playwright-test-token"),
		PrivyUsers:            demoUsers{},
		Passwordless:          demoPasswordless{},
		Sessions:              lib.NewSessionCodec("playwright-session-secret"),
		InsecureSessionCookie: true,
		AllowedOrigins:        []string{"*"},
		ReceivingAddress:      "0x1111111111111111111111111111111111111111",
		CheckoutBaseURL:       "http://127.0.0.1:4173",
	})
	log.Fatal(http.ListenAndServe("127.0.0.1:4173", h))
}
