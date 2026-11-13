package lib

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestPrivyTokenVerifierChecksSignatureAndClaims(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "test-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"keys":[{"kty":"EC","crv":"P-256","alg":"ES256","use":"sig","kid":%q,"x":%q,"y":%q}]}`,
			kid, encodeInt(key.X), encodeInt(key.Y))
	}))
	defer server.Close()
	verifier, err := NewPrivyTokenVerifier("app-test", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	claims := jwt.RegisteredClaims{
		Issuer: "privy.io", Subject: "did:privy:merchant-123", Audience: jwt.ClaimStrings{"app-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}
	unsigned := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	unsigned.Header["kid"] = kid
	token, err := unsigned.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.Verify(token); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}

	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	for name, signed := range map[string]string{
		"wrong signer":  mustSignJWT(t, other, claims),
		"wrong app":     mustSignJWT(t, key, jwt.RegisteredClaims{Issuer: "privy.io", Subject: "did:privy:merchant-123", Audience: jwt.ClaimStrings{"other-app"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}),
		"wrong issuer":  mustSignJWT(t, key, jwt.RegisteredClaims{Issuer: "attacker", Subject: "did:privy:merchant-123", Audience: jwt.ClaimStrings{"app-test"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}),
		"expired":       mustSignJWT(t, key, jwt.RegisteredClaims{Issuer: "privy.io", Subject: "did:privy:merchant-123", Audience: jwt.ClaimStrings{"app-test"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute))}),
		"wrong subject": mustSignJWT(t, key, jwt.RegisteredClaims{Issuer: "privy.io", Subject: "not-a-privy-id", Audience: jwt.ClaimStrings{"app-test"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}),
	} {
		t.Run(name, func(t *testing.T) {
			if verifier.Verify(signed) == nil {
				t.Fatal("invalid token accepted")
			}
		})
	}
}

func mustSignJWT(t *testing.T, key *ecdsa.PrivateKey, claims jwt.RegisteredClaims) string {
	t.Helper()
	unsigned := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	unsigned.Header["kid"] = "test-key"
	token, err := unsigned.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func encodeInt(n *big.Int) string {
	bytes := n.FillBytes(make([]byte, 32))
	return base64.RawURLEncoding.EncodeToString(bytes)
}

type fakePrivyVerifier struct {
	subject string
	err     error
}

func (f fakePrivyVerifier) Verify(string) error            { return f.err }
func (f fakePrivyVerifier) Subject(string) (string, error) { return f.subject, f.err }

type fakePrivyUserReader struct {
	id   string
	user any
	err  error
}

func (f *fakePrivyUserReader) GetUser(_ context.Context, id string) (any, error) {
	f.id = id
	return f.user, f.err
}

func TestGetPrivyProfileUsesVerifiedSubject(t *testing.T) {
	reader := &fakePrivyUserReader{user: map[string]any{"id": "did:privy:merchant-123"}}
	_, _, handler := newTestHandler(t, func(cfg *HandlerConfig) {
		cfg.Verifier = fakePrivyVerifier{subject: "did:privy:merchant-123"}
		cfg.PrivyUsers = reader
	})
	rec := doRequest(t, handler, http.MethodGet, "/v1/auth/me", "", map[string]string{"Authorization": "Bearer privy-token"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if reader.id != "did:privy:merchant-123" {
		t.Fatalf("looked up user %q", reader.id)
	}
}

func TestGetPrivyProfileRejectsLegacyTokenIdentity(t *testing.T) {
	reader := &fakePrivyUserReader{}
	_, _, handler := newTestHandler(t, func(cfg *HandlerConfig) { cfg.PrivyUsers = reader })
	rec := doRequest(t, handler, http.MethodGet, "/v1/auth/me", "", authed(nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want unauthorized", rec.Code)
	}
	if reader.id != "" {
		t.Fatal("profile lookup ran without verified identity")
	}
}
