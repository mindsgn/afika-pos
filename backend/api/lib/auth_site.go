package lib

import (
	"context"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// emailPattern is a deliberately loose check: Privy is the authority on whether
// an address is deliverable, and the form is re-rendered on any error.
var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]{2,}$`)

const authPageCSS = `<style>
.auth{max-width:440px;margin:48px auto;background:var(--paper);border:1px solid var(--line);border-radius:24px;padding:34px}
.auth h1{font-size:30px;letter-spacing:-.04em;margin:0 0 6px}
.auth p.sub{color:var(--muted);margin:0 0 22px;line-height:1.55}
.auth label{display:block;font-size:13px;font-weight:650;margin:0 0 6px;color:#4b4e56}
.auth input{width:100%;min-height:50px;border:1px solid var(--line);border-radius:14px;padding:0 14px;font-size:16px;font-family:inherit;background:var(--canvas);color:var(--ink)}
.auth input:focus{outline:2px solid var(--lime-line);outline-offset:1px}
.auth button{margin-top:18px;width:100%;min-height:50px;border:0;border-radius:14px;background:var(--ink);color:var(--lime);font-size:16px;font-weight:700;font-family:inherit;cursor:pointer}
.auth .alt{margin:16px 0 0;font-size:14px;color:var(--muted)}
.auth .error{margin:0 0 18px;padding:12px 14px;border-radius:14px;background:#FBE5E6;color:var(--red);font-weight:600}
.profile{max-width:560px;margin:48px auto;background:var(--paper);border:1px solid var(--line);border-radius:24px;padding:34px}
.profile h1{font-size:30px;letter-spacing:-.04em;margin:0 0 6px}
.profile dl{display:grid;grid-template-columns:150px 1fr;gap:10px 18px;margin:22px 0 0}
.profile dt{color:var(--muted);font-size:14px}
.profile dd{margin:0;font-weight:600;word-break:break-all}
.profile form{margin-top:26px}
.profile button{min-height:46px;padding:0 20px;border:1px solid var(--line);border-radius:14px;background:var(--paper);color:var(--ink);font-family:inherit;font-weight:650;font-size:15px;cursor:pointer}
</style>`

// privyProfile is the subset of the Privy user we render on the account page.
type privyProfile struct {
	ID             string `json:"id"`
	CreatedAt      int64  `json:"created_at"`
	LinkedAccounts []struct {
		Type    string `json:"type"`
		Address string `json:"address"`
	} `json:"linked_accounts"`
}

func (p privyProfile) email() string {
	for _, account := range p.LinkedAccounts {
		if account.Type == "email" && account.Address != "" {
			return account.Address
		}
	}
	return ""
}

// serveAuthSite handles the server-rendered sign-in and account pages. It runs
// before the static marketing site and reports whether it handled the request.
func (h *Handler) serveAuthSite(w http.ResponseWriter, r *http.Request) bool {
	path := strings.TrimRight(r.URL.Path, "/")
	if path == "" {
		path = "/"
	}
	switch path {
	case "/signin", "/signin/verify", "/signout", "/account":
	default:
		return false
	}

	if h.passwordless == nil || h.sessions == nil {
		h.writeAuthPage(w, r, http.StatusServiceUnavailable, signInPage("email", "", "Website sign-in is not configured on this deployment."))
		return true
	}
	if r.Method == http.MethodPost {
		// Keep form parsing within the same budget as the JSON endpoints.
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	}

	switch path {
	case "/signin":
		if r.Method == http.MethodGet {
			if _, ok := h.sessionUser(r); ok {
				http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
				return true
			}
			step := "email"
			if r.URL.Query().Get("step") == "code" {
				step = "code"
			}
			h.writeAuthPage(w, r, http.StatusOK, signInPage(step, r.URL.Query().Get("email"), ""))
			return true
		}
		if r.Method != http.MethodPost {
			return h.methodNotAllowedPage(w, r, "GET, POST")
		}
		h.handleSignInStart(w, r)
		return true
	case "/signin/verify":
		if r.Method != http.MethodPost {
			return h.methodNotAllowedPage(w, r, "POST")
		}
		h.handleSignInVerify(w, r)
		return true
	case "/signout":
		if r.Method != http.MethodPost {
			return h.methodNotAllowedPage(w, r, "POST")
		}
		h.clearSessionCookie(w)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return true
	default: // /account
		if r.Method != http.MethodGet {
			return h.methodNotAllowedPage(w, r, "GET")
		}
		h.handleAccount(w, r)
		return true
	}
}

func (h *Handler) methodNotAllowedPage(w http.ResponseWriter, r *http.Request, allow string) bool {
	w.Header().Set("Allow", allow)
	h.writeAuthPage(w, r, http.StatusMethodNotAllowed, signInPage("email", "", "That request is not allowed."))
	return true
}

func (h *Handler) handleSignInStart(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.FormValue("email"))
	if !emailPattern.MatchString(email) {
		h.writeAuthPage(w, r, http.StatusBadRequest, signInPage("email", email, "Enter a valid email address."))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()
	if err := h.passwordless.SendCode(ctx, email); err != nil {
		h.logf("privy passwordless init: %v", err)
		h.writeAuthPage(w, r, http.StatusBadGateway, signInPage("email", email, "We could not send a code to that address. Please try again."))
		return
	}
	h.writeAuthPage(w, r, http.StatusOK, signInPage("code", email, ""))
}

func (h *Handler) handleSignInVerify(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.FormValue("email"))
	code := strings.TrimSpace(r.FormValue("code"))
	if !emailPattern.MatchString(email) || code == "" {
		h.writeAuthPage(w, r, http.StatusBadRequest, signInPage("code", email, "Enter the code from your email."))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()
	session, err := h.passwordless.LoginWithCode(ctx, email, code)
	if err != nil {
		h.logf("privy passwordless authenticate: %v", err)
		h.writeAuthPage(w, r, http.StatusUnauthorized, signInPage("code", email, "That code was not accepted. Request a new one and try again."))
		return
	}
	h.setSessionCookie(w, session.UserID)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *Handler) handleAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.sessionUser(r)
	if !ok {
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return
	}
	if h.privyUsers == nil {
		h.writeAuthPage(w, r, http.StatusServiceUnavailable, accountPage(privyProfile{ID: userID}, "Profile lookup is not configured."))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()
	user, err := h.privyUsers.GetUser(ctx, userID)
	if err != nil {
		h.logf("get Privy user profile: %v", err)
		h.writeAuthPage(w, r, http.StatusBadGateway, accountPage(privyProfile{ID: userID}, "We could not load your profile right now."))
		return
	}
	h.writeAuthPage(w, r, http.StatusOK, accountPage(profileFromUser(userID, user), ""))
}

// profileFromUser adapts the Privy SDK's user value to the fields the account
// page shows. Marshalling keeps the API decoupled from the SDK's Go types.
func profileFromUser(userID string, user any) privyProfile {
	profile := privyProfile{ID: userID}
	encoded, err := json.Marshal(user)
	if err != nil {
		return profile
	}
	_ = json.Unmarshal(encoded, &profile)
	if profile.ID == "" {
		profile.ID = userID
	}
	return profile
}

func (h *Handler) sessionUser(r *http.Request) (string, bool) {
	if h.sessions == nil {
		return "", false
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return "", false
	}
	userID, err := h.sessions.Decode(cookie.Value, time.Now())
	if err != nil {
		return "", false
	}
	return userID, true
}

func (h *Handler) setSessionCookie(w http.ResponseWriter, userID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    h.sessions.Encode(userID, time.Now().Add(sessionTTL)),
		Path:     "/",
		HttpOnly: true,
		Secure:   !h.insecureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

func (h *Handler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   !h.insecureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (h *Handler) writeAuthPage(w http.ResponseWriter, r *http.Request, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.WriteString(w, body)
}

func signInPage(step, email, errMsg string) string {
	if step != "code" {
		step = "email"
	}
	escapedEmail := template.HTMLEscapeString(email)
	var form, title, sub string
	if step == "email" {
		title = "Sign in to Afika"
		sub = "Enter your email and we will send you a one-time code."
		form = `<form method="post" action="/signin"><label for="email">Email</label>` +
			`<input id="email" name="email" type="email" autocomplete="email" inputmode="email" required value="` + escapedEmail + `">` +
			`<button type="submit">Send code</button></form>`
	} else {
		title = "Check your email"
		sub = "We sent a 6-digit code to " + escapedEmail + "."
		form = `<form method="post" action="/signin/verify"><input type="hidden" name="email" value="` + escapedEmail + `">` +
			`<label for="code">Authentication code</label>` +
			`<input id="code" name="code" type="text" autocomplete="one-time-code" inputmode="numeric" pattern="[0-9]*" maxlength="6" required autofocus>` +
			`<button type="submit">Verify and continue</button>` +
			`<p class="alt"><a href="/signin?email=` + url.QueryEscape(email) + `">Use a different email</a></p></form>`
	}
	body := siteNav() + authPageCSS + `<main class="wrap"><section class="auth"><h1>` + title + `</h1><p class="sub">` + sub + `</p>` +
		errorBlock(errMsg) + form + `</section></main>` + siteFooter()
	return page(title+" — Afika POS", "Sign in to your Afika POS account with a one-time email code.", "/signin", body)
}

func accountPage(profile privyProfile, errMsg string) string {
	email := profile.email()
	if email == "" {
		email = "—"
	}
	created := "—"
	if profile.CreatedAt > 0 {
		created = time.Unix(profile.CreatedAt, 0).UTC().Format("2 January 2006")
	}
	body := siteNav() + authPageCSS + `<main class="wrap"><section class="profile"><h1>Your account</h1>` +
		`<p class="sub">You are signed in to Afika POS on the web.</p>` + errorBlock(errMsg) +
		`<dl><dt>Email</dt><dd>` + template.HTMLEscapeString(email) + `</dd>` +
		`<dt>Privy user</dt><dd>` + template.HTMLEscapeString(profile.ID) + `</dd>` +
		`<dt>Member since</dt><dd>` + template.HTMLEscapeString(created) + `</dd></dl>` +
		`<form method="post" action="/signout"><button type="submit">Sign out</button></form>` +
		`</section></main>` + siteFooter()
	return page("Your account — Afika POS", "Your Afika POS account details.", "/account", body)
}

func errorBlock(message string) string {
	if strings.TrimSpace(message) == "" {
		return ""
	}
	return `<p class="error">` + template.HTMLEscapeString(message) + `</p>`
}
