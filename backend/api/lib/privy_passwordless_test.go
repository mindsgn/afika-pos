package lib

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPrivyPasswordlessSendCodePostsEmail(t *testing.T) {
	var gotPath, gotAppID, gotClientID string
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAppID = r.Header.Get("privy-app-id")
		gotClientID = r.Header.Get("privy-client-id")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewPrivyPasswordlessClient("app-test", "client-test")
	client.baseURL = server.URL
	if err := client.SendCode(context.Background(), "merchant@example.com"); err != nil {
		t.Fatalf("SendCode: %v", err)
	}

	if gotPath != "/api/v1/passwordless/init" {
		t.Errorf("path = %q, want /api/v1/passwordless/init", gotPath)
	}
	if gotAppID != "app-test" || gotClientID != "client-test" {
		t.Errorf("headers = app-id %q, client-id %q", gotAppID, gotClientID)
	}
	if gotBody["email"] != "merchant@example.com" {
		t.Errorf("body email = %q", gotBody["email"])
	}
}

func TestPrivyPasswordlessLoginWithCodeReturnsSession(t *testing.T) {
	var gotPath string
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"access-token","user":{"id":"did:privy:merchant-1"}}`))
	}))
	defer server.Close()

	client := NewPrivyPasswordlessClient("app-test", "")
	client.baseURL = server.URL
	session, err := client.LoginWithCode(context.Background(), "merchant@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithCode: %v", err)
	}

	if gotPath != "/api/v1/passwordless/authenticate" {
		t.Errorf("path = %q, want /api/v1/passwordless/authenticate", gotPath)
	}
	if gotBody["code"] != "123456" || gotBody["email"] != "merchant@example.com" {
		t.Errorf("body = %#v", gotBody)
	}
	if session.UserID != "did:privy:merchant-1" || session.AccessToken != "access-token" {
		t.Errorf("session = %#v", session)
	}
}

func TestPrivyPasswordlessRejectsErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewPrivyPasswordlessClient("app-test", "")
	client.baseURL = server.URL
	if err := client.SendCode(context.Background(), "merchant@example.com"); err == nil {
		t.Fatal("non-2xx response was accepted")
	}
	if _, err := client.LoginWithCode(context.Background(), "merchant@example.com", "000000"); err == nil {
		t.Fatal("non-2xx response was accepted")
	}
}

func TestPrivyPasswordlessRejectsIncompleteSession(t *testing.T) {
	for name, body := range map[string]string{
		"missing user":  `{"token":"access-token"}`,
		"missing token": `{"user":{"id":"did:privy:merchant-1"}}`,
		"foreign id":    `{"token":"access-token","user":{"id":"user-1"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()

			client := NewPrivyPasswordlessClient("app-test", "")
			client.baseURL = server.URL
			if _, err := client.LoginWithCode(context.Background(), "merchant@example.com", "123456"); err == nil {
				t.Fatal("incomplete session was accepted")
			}
		})
	}
}

func TestPrivyPasswordlessRequiresAppID(t *testing.T) {
	client := NewPrivyPasswordlessClient("", "")
	if err := client.SendCode(context.Background(), "merchant@example.com"); err == nil {
		t.Fatal("missing app id was accepted")
	}
	var nilClient *PrivyPasswordlessClient
	if err := nilClient.SendCode(context.Background(), "merchant@example.com"); err == nil {
		t.Fatal("nil client was accepted")
	}
}
