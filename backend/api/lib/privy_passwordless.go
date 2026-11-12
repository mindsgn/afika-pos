package lib

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultPrivyAuthBaseURL = "https://auth.privy.io"

// PasswordlessClient starts and completes a Privy email one-time-code sign-in.
// Privy's passwordless routes are public client endpoints: they need the app id
// (and, for apps with a client id, that too) but not the app secret.
type PasswordlessClient interface {
	SendCode(ctx context.Context, email string) error
	LoginWithCode(ctx context.Context, email, code string) (PasswordlessSession, error)
}

// PasswordlessSession is the authenticated identity returned by Privy.
type PasswordlessSession struct {
	UserID      string
	AccessToken string
}

// PrivyPasswordlessClient calls Privy's auth API over HTTP.
type PrivyPasswordlessClient struct {
	appID      string
	clientID   string
	baseURL    string
	httpClient *http.Client
}

// NewPrivyPasswordlessClient builds a client for one Privy app.
func NewPrivyPasswordlessClient(appID, clientID string) *PrivyPasswordlessClient {
	return &PrivyPasswordlessClient{
		appID:      strings.TrimSpace(appID),
		clientID:   strings.TrimSpace(clientID),
		baseURL:    defaultPrivyAuthBaseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type privyPasswordlessResponse struct {
	Token string `json:"token"`
	User  struct {
		ID string `json:"id"`
	} `json:"user"`
}

func (c *PrivyPasswordlessClient) post(ctx context.Context, path string, body any, out any) error {
	if c == nil || c.appID == "" {
		return errors.New("Privy app ID is not configured")
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode Privy request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("build Privy request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("privy-app-id", c.appID)
	if c.clientID != "" {
		req.Header.Set("privy-client-id", c.clientID)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call Privy: %w", err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read Privy response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("Privy returned status %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("decode Privy response: %w", err)
	}
	return nil
}

// SendCode asks Privy to email a one-time code to the address.
func (c *PrivyPasswordlessClient) SendCode(ctx context.Context, email string) error {
	return c.post(ctx, "/api/v1/passwordless/init", map[string]string{"email": email}, nil)
}

// LoginWithCode exchanges the emailed code for a session.
func (c *PrivyPasswordlessClient) LoginWithCode(ctx context.Context, email, code string) (PasswordlessSession, error) {
	var response privyPasswordlessResponse
	if err := c.post(ctx, "/api/v1/passwordless/authenticate", map[string]string{"email": email, "code": code}, &response); err != nil {
		return PasswordlessSession{}, err
	}
	if !strings.HasPrefix(response.User.ID, "did:privy:") || response.Token == "" {
		return PasswordlessSession{}, errors.New("Privy returned an incomplete session")
	}
	return PasswordlessSession{UserID: response.User.ID, AccessToken: response.Token}, nil
}
