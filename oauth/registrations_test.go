package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"uuid"

	"hitkeep/api"
	"hitkeep/auth"
	"hitkeep/database"
)

func managedPending(t *testing.T, f *providerFixture, c *ClientRegistration) (*http.Request, *api.OAuthConsent, string) {
	t.Helper()
	verifier := strings.Repeat("v", 64)
	challenge := sha256.Sum256([]byte(verifier))
	query := url.Values{"response_type": {"code"}, "client_id": {c.ID}, "redirect_uri": {c.RedirectURIs[0]}, "resource": {f.p.Resource("api")}, "scope": {"sites:read analytics:read"}, "state": {"managed-client-state"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}}
	w := httptest.NewRecorder()
	f.p.Authorize(w, httptest.NewRequest(http.MethodGet, f.p.Issuer()+"/oauth/authorize?"+query.Encode(), nil))
	if w.Code != 303 {
		t.Fatalf("managed authorization: %d %s", w.Code, w.Body.String())
	}
	request := httptest.NewRequest(http.MethodPost, f.p.Issuer()+"/api/oauth/consent", nil)
	for _, cookie := range w.Result().Cookies() {
		request.AddCookie(cookie)
	}
	consent, err := f.p.Consent(t.Context(), request, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if consent.RegistrationTeamID != f.teamID {
		t.Fatal("managed consent lost team binding")
	}
	return request, consent, verifier
}

func managedCode(t *testing.T, f *providerFixture, c *ClientRegistration) url.Values {
	t.Helper()
	request, consent, verifier := managedPending(t, f, c)
	response := httptest.NewRecorder()
	if err := f.p.CompleteConsent(response, request, f.userID, f.teamID, []uuid.UUID{f.siteID}, "approve", consent.CSRFToken, func(context.Context, *auth.OAuthGrant, uuid.UUID) error { return nil }); err != nil {
		t.Fatal(err)
	}
	callback, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	form := f.codeForm(callback.Query().Get("code"), verifier)
	form.Set("client_id", c.ID)
	form.Set("redirect_uri", c.RedirectURIs[0])
	return form
}

func TestManagedOAuthClientLifecycleAndPendingInvalidation(t *testing.T) {
	f := newProviderFixture(t)
	ctx := t.Context()
	client, err := f.p.SaveClient(ctx, f.userID, f.teamID, ClientRegistration{Name: "Reporting app", RedirectURIs: []string{"https://reports.example/callback"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	request, consent, _ := managedPending(t, f, client)
	if err := f.p.CompleteConsent(httptest.NewRecorder(), request, f.userID, uuid.NewV7(), []uuid.UUID{f.siteID}, "approve", consent.CSRFToken, func(context.Context, *auth.OAuthGrant, uuid.UUID) error { return nil }); !errors.Is(err, ErrConsentForbidden) {
		t.Fatalf("foreign consent allowed: %v", err)
	}
	form := managedCode(t, f, client)
	status, tokens := f.token(t, form)
	requireToken(t, status, tokens)
	pending, info, _ := managedPending(t, f, client)
	oldCode := managedCode(t, f, client)
	client.Name = "Renamed"
	if _, err := f.p.SaveClient(ctx, f.userID, f.teamID, *client, false); err != nil {
		t.Fatal(err)
	}
	if err := f.p.CompleteConsent(httptest.NewRecorder(), pending, f.userID, f.teamID, []uuid.UUID{f.siteID}, "approve", info.CSRFToken, func(context.Context, *auth.OAuthGrant, uuid.UUID) error { return nil }); !errors.Is(err, ErrInvalidConsent) {
		t.Fatalf("old pending consent accepted: %v", err)
	}
	if _, err := f.p.Authenticate(ctx, tokens.AccessToken, f.p.Resource("api")); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("edit retained access: %v", err)
	}
	if status, _ := f.token(t, oldCode); status != 400 {
		t.Fatalf("edit retained code: %d", status)
	}
	refresh := f.refreshForm(tokens.RefreshToken)
	refresh.Set("client_id", client.ID)
	if status, _ := f.token(t, refresh); status != 400 {
		t.Fatalf("edit retained refresh: %d", status)
	}
	// A new provider uses durable registrations, with no in-memory registry update.
	f.p, err = New(f.store, f.options)
	if err != nil {
		t.Fatal(err)
	}
	status, tokens = f.token(t, managedCode(t, f, client))
	requireToken(t, status, tokens)
	pending, info, _ = managedPending(t, f, client)
	if err := f.p.RevokeClient(ctx, f.userID, f.teamID, client.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.p.Authenticate(ctx, tokens.AccessToken, f.p.Resource("api")); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("revoke retained access: %v", err)
	}
	if _, err := f.p.resolveClient(ctx, client.ID); !errors.Is(err, database.ErrOAuthNotFound) {
		t.Fatalf("revoked client resolved: %v", err)
	}
	if err := f.p.CompleteConsent(httptest.NewRecorder(), pending, f.userID, f.teamID, []uuid.UUID{f.siteID}, "approve", info.CSRFToken, func(context.Context, *auth.OAuthGrant, uuid.UUID) error { return nil }); err == nil {
		t.Fatal("revoked pending consent accepted")
	}
	clients, err := f.p.ListClients(ctx, f.teamID)
	if err != nil || len(clients) != 0 {
		t.Fatalf("revoked client listed: %v %v", clients, err)
	}
	// Existing configured clients and their consent/token flow remain supported.
	code, verifier := f.authorize(t)
	status, tokens = f.token(t, f.codeForm(code, verifier))
	requireToken(t, status, tokens)
}

func TestManagedOAuthClientValidationAndIsolation(t *testing.T) {
	f := newProviderFixture(t)
	for _, redirect := range []string{"https://*.example.com/callback", "http://example.com/callback", "http://127.0.0.1:1234/callback", "https://example.com/#fragment", "https://user:pass@example.com/callback", "https://example.com:0/callback"} {
		if _, err := f.p.SaveClient(t.Context(), f.userID, f.teamID, ClientRegistration{Name: "Client", RedirectURIs: []string{redirect}}, true); !errors.Is(err, ErrInvalidConsent) {
			t.Fatalf("invalid redirect allowed: %s %v", redirect, err)
		}
	}
	client, err := f.p.SaveClient(t.Context(), f.userID, f.teamID, ClientRegistration{Name: "Client", RedirectURIs: []string{"https://example.com/callback"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	otherTeam := uuid.NewV7()
	if _, err := f.p.SaveClient(t.Context(), f.userID, otherTeam, *client, false); !errors.Is(err, database.ErrOAuthNotFound) {
		t.Fatalf("foreign edit allowed: %v", err)
	}
	if err := f.p.RevokeClient(t.Context(), f.userID, otherTeam, client.ID); !errors.Is(err, database.ErrOAuthNotFound) {
		t.Fatalf("foreign revoke allowed: %v", err)
	}
	f.leader.Store(false)
	if _, err := f.p.SaveClient(t.Context(), f.userID, f.teamID, *client, false); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("follower accepted edit: %v", err)
	}
}
