package lib

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type fakePasswordless struct {
	sentEmail string
	sendErr   error
	gotEmail  string
	gotCode   string
	session   PasswordlessSession
	loginErr  error
}

func (f *fakePasswordless) SendCode(_ context.Context, email string) error {
	f.sentEmail = email
	return f.sendErr
}

func (f *fakePasswordless) LoginWithCode(_ context.Context, email, code string) (PasswordlessSession, error) {
	f.gotEmail = email
	f.gotCode = code
	return f.session, f.loginErr
}

func authSiteHandler(t *testing.T, passwordless PasswordlessClient, users PrivyUserReader) http.Handler {
	t.Helper()
	_, _, handler := newTestHandler(t, func(cfg *HandlerConfig) {
		cfg.Passwordless = passwordless
		cfg.Sessions = NewSessionCodec("test-secret")
		cfg.PrivyUsers = users
	})
	return handler
}

// doForm posts an application/x-www-form-urlencoded body, which is what the
// sign-in forms submit.
func doForm(t *testing.T, h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			return cookie
		}
	}
	t.Fatalf("response did not set %s", sessionCookieName)
	return nil
}

func TestSignInFlowIssuesSessionAndShowsAccount(t *testing.T) {
	passwordless := &fakePasswordless{session: PasswordlessSession{UserID: "did:privy:merchant-1", AccessToken: "token"}}
	users := &fakePrivyUserReader{user: map[string]any{
		"id":         "did:privy:merchant-1",
		"created_at": 1735689600,
		"linked_accounts": []map[string]any{
			{"type": "email", "address": "merchant@example.com"},
		},
	}}
	handler := authSiteHandler(t, passwordless, users)

	// Step one: email -> send the code.
	rec := doForm(t, handler, "/signin", url.Values{"email": {"merchant@example.com"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("send code status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if passwordless.sentEmail != "merchant@example.com" {
		t.Fatalf("SendCode got %q", passwordless.sentEmail)
	}
	if !strings.Contains(rec.Body.String(), `action="/signin/verify"`) {
		t.Fatal("code step was not rendered")
	}

	// Step two: code -> session cookie and redirect to the account page.
	rec = doForm(t, handler, "/signin/verify", url.Values{"email": {"merchant@example.com"}, "code": {"123456"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("verify status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "/account" {
		t.Fatalf("Location = %q, want /account", location)
	}
	if passwordless.gotCode != "123456" {
		t.Fatalf("LoginWithCode got code %q", passwordless.gotCode)
	}
	cookie := sessionCookie(t, rec)
	if !cookie.HttpOnly {
		t.Error("session cookie is not HttpOnly")
	}

	// Step three: the cookie unlocks the account page.
	req := httptest.NewRequest(http.MethodGet, "/account", nil)
	req.AddCookie(cookie)
	account := httptest.NewRecorder()
	handler.ServeHTTP(account, req)
	if account.Code != http.StatusOK {
		t.Fatalf("account status = %d, body = %s", account.Code, account.Body.String())
	}
	if !strings.Contains(account.Body.String(), "merchant@example.com") {
		t.Fatal("account page does not show the profile email")
	}
	if users.id != "did:privy:merchant-1" {
		t.Fatalf("profile lookup used %q", users.id)
	}
}

func TestSignInRejectsInvalidEmail(t *testing.T) {
	passwordless := &fakePasswordless{}
	handler := authSiteHandler(t, passwordless, nil)
	rec := doForm(t, handler, "/signin", url.Values{"email": {"not-an-email"}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if passwordless.sentEmail != "" {
		t.Fatal("a code was sent for an invalid email")
	}
}

func TestSignInReportsPrivyFailure(t *testing.T) {
	passwordless := &fakePasswordless{sendErr: errors.New("privy down")}
	handler := authSiteHandler(t, passwordless, nil)
	rec := doForm(t, handler, "/signin", url.Values{"email": {"merchant@example.com"}})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestSignInVerifyRejectsBadCode(t *testing.T) {
	passwordless := &fakePasswordless{loginErr: errors.New("bad code")}
	handler := authSiteHandler(t, passwordless, nil)
	rec := doForm(t, handler, "/signin/verify", url.Values{"email": {"merchant@example.com"}, "code": {"000000"}})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == sessionCookieName && cookie.Value != "" {
			t.Fatal("a session was issued for a rejected code")
		}
	}
}

func TestSignInUnconfiguredReturnsServiceUnavailable(t *testing.T) {
	_, _, handler := newTestHandler(t)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/signin", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(rec.Body.String(), "not configured") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestAccountRedirectsWhenSignedOut(t *testing.T) {
	handler := authSiteHandler(t, &fakePasswordless{}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/account", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != "/signin" {
		t.Fatalf("Location = %q, want /signin", location)
	}
}

func TestSignInGetRedirectsWhenAlreadySignedIn(t *testing.T) {
	handler := authSiteHandler(t, &fakePasswordless{}, nil)
	cookie := NewSessionCodec("test-secret").Encode("did:privy:merchant-1", time.Now().Add(time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/signin", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != "/account" {
		t.Fatalf("Location = %q, want /account", location)
	}
}

func TestSignOutClearsSession(t *testing.T) {
	handler := authSiteHandler(t, &fakePasswordless{}, nil)
	cookie := NewSessionCodec("test-secret").Encode("did:privy:merchant-1", time.Now().Add(time.Hour))
	req := httptest.NewRequest(http.MethodPost, "/signout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	cleared := sessionCookie(t, rec)
	if cleared.Value != "" || cleared.MaxAge >= 0 {
		t.Fatalf("session cookie was not cleared: %#v", cleared)
	}
}
