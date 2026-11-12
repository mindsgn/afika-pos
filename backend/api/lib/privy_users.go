package lib

import (
	"context"
	"errors"
	"net/http"
	"strings"

	privy "github.com/privy-io/go-sdk"
)

// PrivyUserReader provides the authenticated user's Privy profile. The Go SDK
// is used for the Privy Users API; it is not a local JWT verifier.
type PrivyUserReader interface {
	GetUser(context.Context, string) (any, error)
}

type PrivyUserAPI struct{ client *privy.PrivyClient }

func NewPrivyUserAPI(appID, appSecret string) (*PrivyUserAPI, error) {
	if strings.TrimSpace(appID) == "" || strings.TrimSpace(appSecret) == "" {
		return nil, errors.New("Privy app ID and app secret are required")
	}
	return &PrivyUserAPI{client: privy.NewPrivyClient(privy.PrivyClientOptions{AppID: appID, AppSecret: appSecret})}, nil
}

func (p *PrivyUserAPI) GetUser(ctx context.Context, userID string) (any, error) {
	return p.client.Users.Get(ctx, userID)
}

func (h *Handler) getPrivyProfile(_ http.ResponseWriter, r *http.Request) (int, any, *apiError) {
	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		return 0, nil, unauthenticated()
	}
	identityVerifier, ok := h.verifier.(interface{ Subject(string) (string, error) })
	if !ok {
		return 0, nil, unauthenticated()
	}
	userID, err := identityVerifier.Subject(token)
	if err != nil {
		return 0, nil, unauthenticated()
	}
	if h.privyUsers == nil {
		return 0, nil, &apiError{status: http.StatusServiceUnavailable, code: "PRIVY_NOT_CONFIGURED", message: "Privy profile lookup is not configured."}
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()
	user, err := h.privyUsers.GetUser(ctx, userID)
	if err != nil {
		h.logf("get Privy user profile: %v", err)
		return 0, nil, internalError()
	}
	return http.StatusOK, struct {
		Data any `json:"data"`
	}{Data: user}, nil
}
