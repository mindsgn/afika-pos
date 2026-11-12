package lib

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"strings"
)

// ErrUnauthenticated is returned for a missing, malformed, or incorrect token.
var ErrUnauthenticated = errors.New("unauthenticated")

// TokenVerifier turns a bearer token into an authentication decision. The API
// does not derive a user identity from the token: wallet, balance, and
// transaction lookups are keyed by the address supplied in the request.
type TokenVerifier interface {
	Verify(token string) error
}

// StaticTokenVerifier checks a shared API token configured through the
// API_TOKEN environment variable. Comparison is constant time.
type StaticTokenVerifier struct {
	token string
}

// NewStaticTokenVerifier builds a verifier for one shared token. An empty
// token disables authentication and rejects every request.
func NewStaticTokenVerifier(token string) *StaticTokenVerifier {
	return &StaticTokenVerifier{token: strings.TrimSpace(token)}
}

// Verify reports whether token matches the configured API token.
func (v *StaticTokenVerifier) Verify(token string) error {
	if v.token == "" || token == "" {
		return ErrUnauthenticated
	}

	// Hash both values so the comparison does not leak the token length.
	got := sha256.Sum256([]byte(token))
	want := sha256.Sum256([]byte(v.token))
	if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
		return ErrUnauthenticated
	}
	return nil
}

// bearerToken extracts the token from an Authorization header.
func bearerToken(header string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(header))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "bearer") || fields[1] == "" {
		return "", false
	}
	return fields[1], true
}
