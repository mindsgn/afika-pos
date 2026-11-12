//go:build js && wasm

package main

import (
	"context"
	"errors"

	"github.com/mindsgn-studio/afika-backend/api/lib"
)

// These fakes are enabled only by the local Playwright D1 harness.
type demoPasswordless struct{}

func (demoPasswordless) SendCode(context.Context, string) error { return nil }

func (demoPasswordless) LoginWithCode(_ context.Context, _ string, code string) (lib.PasswordlessSession, error) {
	if code != "000000" {
		return lib.PasswordlessSession{}, errors.New("invalid code")
	}
	return lib.PasswordlessSession{UserID: "did:privy:e2e-merchant", AccessToken: "e2e-access-token"}, nil
}

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
