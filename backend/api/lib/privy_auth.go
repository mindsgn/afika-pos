package lib

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const privyJWKSCacheTTL = 5 * time.Minute

// PrivyTokenVerifier validates Privy app access tokens using the app's JWKS.
type PrivyTokenVerifier struct {
	appID      string
	jwksURL    string
	httpClient *http.Client
	mu         sync.Mutex
	keys       map[string]*ecdsa.PublicKey
	keysUntil  time.Time
}

func NewPrivyTokenVerifier(appID, jwksURL string) (*PrivyTokenVerifier, error) {
	appID = strings.TrimSpace(appID)
	jwksURL = strings.TrimSpace(jwksURL)
	if appID == "" {
		return nil, errors.New("Privy app ID is required")
	}
	if jwksURL == "" {
		return nil, errors.New("Privy JWKS URL is required")
	}
	return &PrivyTokenVerifier{
		appID: appID, jwksURL: jwksURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (v *PrivyTokenVerifier) Verify(token string) error {
	_, err := v.Subject(token)
	return err
}

// Subject returns the authenticated Privy DID after validating the token.
func (v *PrivyTokenVerifier) Subject(token string) (string, error) {
	if v == nil || strings.TrimSpace(token) == "" {
		return "", ErrUnauthenticated
	}
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodES256.Alg() {
			return nil, ErrUnauthenticated
		}
		kid, ok := t.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, ErrUnauthenticated
		}
		return v.publicKey(kid)
	}, jwt.WithValidMethods([]string{jwt.SigningMethodES256.Alg()}), jwt.WithIssuer("privy.io"), jwt.WithAudience(v.appID), jwt.WithExpirationRequired(), jwt.WithTimeFunc(time.Now))
	if err != nil || parsed == nil || !parsed.Valid || !strings.HasPrefix(claims.Subject, "did:privy:") {
		return "", ErrUnauthenticated
	}
	return claims.Subject, nil
}

func (v *PrivyTokenVerifier) publicKey(kid string) (*ecdsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if time.Now().After(v.keysUntil) {
		if err := v.refreshKeys(); err != nil {
			return nil, err
		}
	}
	key := v.keys[kid]
	if key == nil {
		// Refresh once for key rotations before rejecting an unknown kid.
		if err := v.refreshKeys(); err != nil {
			return nil, err
		}
		key = v.keys[kid]
	}
	if key == nil {
		return nil, ErrUnauthenticated
	}
	return key, nil
}

func (v *PrivyTokenVerifier) refreshKeys() error {
	req, err := http.NewRequest(http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("Privy JWKS request failed")
	}
	var set struct {
		Keys []struct {
			Kty string `json:"kty"`
			Crv string `json:"crv"`
			Alg string `json:"alg"`
			Use string `json:"use"`
			Kid string `json:"kid"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return err
	}
	keys := make(map[string]*ecdsa.PublicKey, len(set.Keys))
	for _, item := range set.Keys {
		if item.Kty != "EC" || item.Crv != "P-256" || item.Alg != "ES256" || (item.Use != "" && item.Use != "sig") || item.Kid == "" {
			continue
		}
		x, errX := base64.RawURLEncoding.DecodeString(item.X)
		y, errY := base64.RawURLEncoding.DecodeString(item.Y)
		if errX != nil || errY != nil {
			continue
		}
		pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
		if !pub.Curve.IsOnCurve(pub.X, pub.Y) {
			continue
		}
		keys[item.Kid] = pub
	}
	if len(keys) == 0 {
		return errors.New("Privy JWKS contains no supported ES256 keys")
	}
	v.keys = keys
	v.keysUntil = time.Now().Add(privyJWKSCacheTTL)
	return nil
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
