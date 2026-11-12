package lib

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

const sessionCookieName = "afika_session"

// sessionTTL is how long a signed-in website session stays valid.
const sessionTTL = 24 * time.Hour

// ErrInvalidSession is returned for a missing, malformed, forged, or expired
// session cookie.
var ErrInvalidSession = errors.New("invalid session")

// SessionCodec signs and verifies the website's session cookie. The cookie is
// opaque to the browser and carries only the Privy user id plus an expiry,
// signed with SESSION_SECRET so a client cannot forge a session.
type SessionCodec struct {
	secret []byte
}

// NewSessionCodec returns a codec for the given secret. An empty secret returns
// nil, which disables website sessions.
func NewSessionCodec(secret string) *SessionCodec {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return nil
	}
	return &SessionCodec{secret: []byte(trimmed)}
}

func (c *SessionCodec) sign(payload string) string {
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Encode returns a signed cookie value for userID that expires at expiry.
func (c *SessionCodec) Encode(userID string, expiry time.Time) string {
	payload := userID + "|" + strconv.FormatInt(expiry.Unix(), 10)
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + c.sign(payload)
}

// Decode verifies a cookie value and returns the Privy user id when the
// signature is valid and the session has not expired.
func (c *SessionCodec) Decode(value string, now time.Time) (string, error) {
	if c == nil || strings.TrimSpace(value) == "" {
		return "", ErrInvalidSession
	}
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return "", ErrInvalidSession
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", ErrInvalidSession
	}
	payload := string(raw)
	if !hmac.Equal([]byte(c.sign(payload)), []byte(parts[1])) {
		return "", ErrInvalidSession
	}
	fields := strings.SplitN(payload, "|", 2)
	if len(fields) != 2 || !strings.HasPrefix(fields[0], "did:privy:") {
		return "", ErrInvalidSession
	}
	expiry, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || now.Unix() >= expiry {
		return "", ErrInvalidSession
	}
	return fields[0], nil
}
