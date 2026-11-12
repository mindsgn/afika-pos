package lib

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// PrivyTokenVerifier validates Privy app access tokens with the app's ES256
// verification key. The Go SDK is for Privy's management API; token signature
// verification is performed locally so request authentication does not depend
// on a network call to Privy.
type PrivyTokenVerifier struct {
	appID string
	key   *ecdsa.PublicKey
}

func NewPrivyTokenVerifier(appID, publicKeyPEM string) (*PrivyTokenVerifier, error) {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return nil, errors.New("Privy app ID is required")
	}
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return nil, errors.New("Privy verification key must be PEM encoded")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		if cert, certErr := x509.ParseCertificate(block.Bytes); certErr == nil {
			parsed = cert.PublicKey
		} else {
			return nil, errors.New("could not parse Privy verification key")
		}
	}
	key, ok := parsed.(*ecdsa.PublicKey)
	if !ok || key.Curve.Params().Name != "P-256" {
		return nil, errors.New("Privy verification key must be an ECDSA P-256 public key")
	}
	return &PrivyTokenVerifier{appID: appID, key: key}, nil
}

func (v *PrivyTokenVerifier) Verify(token string) error {
	_, err := v.Subject(token)
	return err
}

// Subject returns the authenticated Privy DID after validating the token.
func (v *PrivyTokenVerifier) Subject(token string) (string, error) {
	if v == nil || v.key == nil || strings.TrimSpace(token) == "" {
		return "", ErrUnauthenticated
	}
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodES256.Alg() {
			return nil, ErrUnauthenticated
		}
		return v.key, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodES256.Alg()}), jwt.WithIssuer("privy.io"), jwt.WithAudience(v.appID), jwt.WithExpirationRequired(), jwt.WithTimeFunc(time.Now))
	if err != nil || parsed == nil || !parsed.Valid || !strings.HasPrefix(claims.Subject, "did:privy:") {
		return "", ErrUnauthenticated
	}
	return claims.Subject, nil
}

// AnyTokenVerifier allows a controlled transition from a legacy server token
// to Privy sessions without invalidating receipt-worker credentials.
type AnyTokenVerifier []TokenVerifier

func (v AnyTokenVerifier) Verify(token string) error {
	for _, verifier := range v {
		if verifier != nil && verifier.Verify(token) == nil {
			return nil
		}
	}
	return ErrUnauthenticated
}

func (v AnyTokenVerifier) Subject(token string) (string, error) {
	for _, verifier := range v {
		if subjectVerifier, ok := verifier.(interface{ Subject(string) (string, error) }); ok {
			if subject, err := subjectVerifier.Subject(token); err == nil {
				return subject, nil
			}
		}
	}
	return "", ErrUnauthenticated
}
