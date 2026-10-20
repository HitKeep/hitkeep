package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"github.com/ory/fosite"

	"hitkeep/auth"
	"hitkeep/database"
)

type providerFixture struct {
	path                   string
	p                      *Provider
	store                  *database.Store
	options                Options
	userID, teamID, siteID uuid.UUID
	leader                 atomic.Bool
}

func newProviderFixture(t *testing.T) *providerFixture {
	t.Helper()
	f := &providerFixture{}
	f.leader.Store(true)
	f.path = filepath.Join(t.TempDir(), "oauth.db")
	store := database.NewStore(f.path)
	if err := store.Connect(); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.store = store
	t.Cleanup(func() { _ = f.store.Close() })
	oldUser, err := store.CreateUser(t.Context(), "oauth@example.test", "unused-test-hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(t.Context(), oldUser, "oauth-site.example")
	if err != nil {
		t.Fatal(err)
	}
	team, err := store.GetSiteTenantID(t.Context(), site.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.userID = uuid.UUID(oldUser)
	f.siteID = uuid.UUID(site.ID)
	f.teamID = uuid.UUID(team)
	f.options = Options{Issuer: "https://hitkeep.example/prefix", MCPPath: "/mcp", MCPEnabled: true, BrowserSigningKey: []byte(strings.Repeat("b", 32)), Clients: []ClientMetadata{{ID: "test-client", Name: "Test client", RedirectURIs: []string{"https://client.example/callback?action=oauth"}, AuthMethod: "none", GrantTypes: []string{"authorization_code", "refresh_token"}}}, IsLeader: f.leader.Load}
	f.p, err = New(store, f.options)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *providerFixture) authorize(t *testing.T) (string, string) {
	t.Helper()
	verifier := strings.Repeat("v", 64)
	challenge := sha256.Sum256([]byte(verifier))
	query := url.Values{"response_type": {"code"}, "client_id": {"test-client"}, "redirect_uri": {"https://client.example/callback?action=oauth"}, "resource": {f.p.Resource("api")}, "scope": {"sites:read analytics:read tracking_domains:read ingest:write"}, "state": {"client-opaque-state"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}}
	r := httptest.NewRequest(http.MethodGet, f.p.Issuer()+"/oauth/authorize?"+query.Encode(), nil)
	w := httptest.NewRecorder()
	f.p.Authorize(w, r)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != f.p.Issuer()+"/oauth/consent" {
		t.Fatalf("authorize did not reach native consent: %d %s %s", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	consentRequest := httptest.NewRequest(http.MethodPost, f.p.Issuer()+"/api/oauth/consent", nil)
	for _, c := range w.Result().Cookies() {
		consentRequest.AddCookie(c)
	}
	consent, err := f.p.Consent(t.Context(), consentRequest, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if consent.Client.CallbackOrigin != "https://client.example" || consent.Client.ID != "test-client" {
		t.Fatalf("unsafe/wrong consent display: %+v", consent.Client)
	}
	completed := httptest.NewRecorder()
	if err := f.p.CompleteConsent(completed, consentRequest, f.userID, f.teamID, []uuid.UUID{f.siteID}, "approve", consent.CSRFToken, func(_ context.Context, g *auth.OAuthGrant, siteID uuid.UUID) error {
		if g.UserID != f.userID || g.TeamID != f.teamID || siteID != f.siteID {
			return errors.New("wrong consent principal")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	callback, err := url.Parse(completed.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	params := callback.Query()
	if completed.Code != 303 || params.Get("iss") != f.p.Issuer() || params.Get("state") != "client-opaque-state" || params.Get("code") == "" {
		t.Fatalf("invalid authorization response: %d %s", completed.Code, completed.Header().Get("Location"))
	}
	return params.Get("code"), verifier
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	Resource     string `json:"resource"`
	Error        string `json:"error"`
	Debug        string `json:"error_debug"`
	Description  string `json:"error_description"`
}

func (f *providerFixture) token(t *testing.T, form url.Values) (int, tokenResponse) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, f.p.Issuer()+"/oauth/token", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	f.p.Token(w, r)
	var result tokenResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid token JSON status %d: %v body %s", w.Code, err, w.Body.String())
	}
	return w.Code, result
}
func (f *providerFixture) codeForm(code, verifier string) url.Values {
	return url.Values{"grant_type": {"authorization_code"}, "client_id": {"test-client"}, "redirect_uri": {"https://client.example/callback?action=oauth"}, "resource": {f.p.Resource("api")}, "code": {code}, "code_verifier": {verifier}}
}
func (f *providerFixture) refreshForm(refresh string) url.Values {
	return url.Values{"grant_type": {"refresh_token"}, "client_id": {"test-client"}, "resource": {f.p.Resource("api")}, "refresh_token": {refresh}}
}
func requireToken(t *testing.T, status int, result tokenResponse) {
	t.Helper()
	if status != 200 || !IsAccessToken(result.AccessToken) || !strings.HasPrefix(result.RefreshToken, refreshTokenPrefix) || !strings.EqualFold(result.TokenType, "Bearer") || result.ExpiresIn <= 0 || result.Scope == "" {
		t.Fatalf("token contract failed: status=%d %+v", status, result)
	}
}

func TestOAuthProviderPKCEBindingRollbackAndSignatureOnly(t *testing.T) {
	f := newProviderFixture(t)
	code, verifier := f.authorize(t)
	for _, change := range []func(url.Values){func(form url.Values) { form.Set("code_verifier", strings.Repeat("x", 64)) }, func(form url.Values) { form.Set("client_id", "other-client") }, func(form url.Values) { form.Set("redirect_uri", "https://evil.example/callback") }, func(form url.Values) { form.Set("resource", f.p.Resource("mcp")) }} {
		form := f.codeForm(code, verifier)
		change(form)
		status, result := f.token(t, form)
		if status != 400 || result.AccessToken != "" {
			t.Fatalf("binding failure minted token: %d %+v", status, result)
		}
	}
	status, result := f.token(t, f.codeForm(code, verifier))
	requireToken(t, status, result)
	grant, err := f.p.Authenticate(t.Context(), result.AccessToken, f.p.Resource("api"))
	if err != nil || grant == nil || grant.UserID != f.userID || grant.TeamID != f.teamID || len(grant.SiteIDs) != 1 {
		t.Fatalf("valid token rejected: %v %+v", err, grant)
	}
	if _, err := f.p.Authenticate(t.Context(), result.AccessToken, f.p.Resource("mcp")); err == nil {
		t.Fatal("cross-resource token accepted")
	}
	rows, err := f.store.DB().QueryContext(t.Context(), "SELECT signature,payload FROM oauth_credentials")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var signature, payload string
		if err := rows.Scan(&signature, &payload); err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{code, verifier, result.AccessToken, result.RefreshToken} {
			if strings.Contains(signature, secret) || strings.Contains(payload, secret) {
				t.Fatal("recoverable credential persisted")
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
func TestOAuthProviderCodeRaceAndReplayRevokesDescendants(t *testing.T) {
	f := newProviderFixture(t)
	code, verifier := f.authorize(t)
	var wg sync.WaitGroup
	responses := make(chan tokenResponse, 2)
	statuses := make(chan int, 2)
	other := &providerFixture{store: f.store, options: f.options, userID: f.userID, teamID: f.teamID, siteID: f.siteID}
	var err error
	other.p, err = New(f.store, f.options)
	if err != nil {
		t.Fatal(err)
	}
	for _, instance := range []*providerFixture{f, other} {
		instance.p.core = delayedCommitProvider{OAuth2Provider: instance.p.core}
		wg.Go(func() {
			status, result := instance.token(t, instance.codeForm(code, verifier))
			if status != 200 && status != 400 {
				t.Errorf("race error: status=%d error=%s debug=%s description=%s", status, result.Error, result.Debug, result.Description)
			}
			statuses <- status
			responses <- result
		})
	}
	wg.Wait()
	close(statuses)
	close(responses)
	success := 0
	for status := range statuses {
		if status == 200 {
			success++
		} else if status != 400 {
			t.Fatalf("unexpected race status %d", status)
		}
	}
	if success != 1 {
		t.Fatalf("authorization code had %d winners", success)
	}
	for response := range responses {
		if response.AccessToken != "" {
			if _, err := f.p.Authenticate(t.Context(), response.AccessToken, f.p.Resource("api")); err == nil {
				t.Fatal("code replay left valid descendant")
			}
		}
	}
}
func TestOAuthProviderRotatingRefreshReuseAndLostResponse(t *testing.T) {
	f := newProviderFixture(t)
	code, verifier := f.authorize(t)
	status, first := f.token(t, f.codeForm(code, verifier))
	requireToken(t, status, first)
	status, rotated := f.token(t, f.refreshForm(first.RefreshToken))
	requireToken(t, status, rotated)
	if rotated.RefreshToken == first.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}
	status, replayed := f.token(t, f.refreshForm(first.RefreshToken))
	if status != 400 || replayed.Error != "invalid_grant" {
		t.Fatalf("refresh replay accepted: %d %+v", status, replayed)
	}
	if _, err := f.p.Authenticate(t.Context(), rotated.AccessToken, f.p.Resource("api")); err == nil {
		t.Fatal("invalid_grant rolled back family revocation")
	}
	code, verifier = f.authorize(t)
	status, first = f.token(t, f.codeForm(code, verifier))
	requireToken(t, status, first)
	status, lost := f.token(t, f.refreshForm(first.RefreshToken))
	requireToken(t, status, lost) // Client loses this already committed response.
	status, _ = f.token(t, f.refreshForm(first.RefreshToken))
	if status != 400 {
		t.Fatal("lost response received replay grace")
	}
	if _, err := f.p.Authenticate(t.Context(), lost.AccessToken, f.p.Resource("api")); err == nil {
		t.Fatal("lost-response retry left descendant active")
	}
}
func TestOAuthProviderRefreshRaceRevokeAndRestart(t *testing.T) {
	f := newProviderFixture(t)
	code, verifier := f.authorize(t)
	status, first := f.token(t, f.codeForm(code, verifier))
	requireToken(t, status, first)
	var wg sync.WaitGroup
	var refreshed tokenResponse
	wg.Go(func() { _, refreshed = f.token(t, f.refreshForm(first.RefreshToken)) })
	wg.Go(func() {
		form := url.Values{"token": {first.RefreshToken}, "client_id": {"test-client"}}
		r := httptest.NewRequest(http.MethodPost, f.p.Issuer()+"/oauth/revoke", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		f.p.Revoke(w, r)
		if w.Code != 200 {
			t.Errorf("revoke failed: %d", w.Code)
		}
	})
	wg.Wait()
	for _, token := range []string{first.AccessToken, refreshed.AccessToken} {
		if token != "" {
			if _, err := f.p.Authenticate(t.Context(), token, f.p.Resource("api")); err == nil {
				t.Fatal("refresh/revoke race left active descendant")
			}
		}
	}
	code, verifier = f.authorize(t)
	status, first = f.token(t, f.codeForm(code, verifier))
	requireToken(t, status, first)
	// A browser-signing-key change invalidates pending browser state, never durable opaque credentials.
	f.options.BrowserSigningKey = []byte(strings.Repeat("c", 32))
	f.options.Clients = nil // Existing flows use their durable snapshot even when registration disappears.
	if err := f.store.Close(); err != nil {
		t.Fatal(err)
	}
	f.store = database.NewStore(f.path)
	if err := f.store.Connect(); err != nil {
		t.Fatal(err)
	}
	p, err := New(f.store, f.options)
	if err != nil {
		t.Fatal(err)
	}
	f.p = p
	status, after := f.token(t, f.refreshForm(first.RefreshToken))
	requireToken(t, status, after)
	f.leader.Store(false)
	if _, err := f.p.Authenticate(t.Context(), after.AccessToken, f.p.Resource("api")); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("non-authoritative node accepted token: %v", err)
	}
	if grant, err := f.p.Authenticate(t.Context(), "legacy-token", f.p.Resource("api")); grant != nil || err != nil {
		t.Fatal("legacy token behavior changed")
	}
}

func TestOAuthMetadataCORSAndDisabledMCP(t *testing.T) {
	urls, err := parseIssuer("https://hitkeep.example/prefix", "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	p := &Provider{urls: urls}
	for _, serve := range []http.HandlerFunc{p.Metadata, p.ProtectedResourceMetadata("api")} {
		w := httptest.NewRecorder()
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		serve(w, httptest.NewRequest(http.MethodGet, "https://hitkeep.example/metadata", nil))
		if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "*" || w.Header().Get("Access-Control-Allow-Credentials") != "" {
			t.Fatalf("public metadata CORS failed: %d %v", w.Code, w.Header())
		}
	}
	if p.Resource("mcp") != "" || p.ResourceMetadataURL("mcp") != "" || len(p.DiscoveryPaths()) != 2 {
		t.Fatal("disabled MCP was advertised")
	}
	w := httptest.NewRecorder()
	p.ProtectedResourceMetadata("mcp")(w, httptest.NewRequest(http.MethodGet, "https://hitkeep.example/metadata", nil))
	if w.Code != 404 {
		t.Fatal("disabled MCP metadata served")
	}
}

func TestOAuthRevocationUsesFositeAndConsumedRefreshRevokesFamily(t *testing.T) {
	f := newProviderFixture(t)
	code, verifier := f.authorize(t)
	status, first := f.token(t, f.codeForm(code, verifier))
	requireToken(t, status, first)
	status, rotated := f.token(t, f.refreshForm(first.RefreshToken))
	requireToken(t, status, rotated)
	// Removing the registered handler must make actual revocation fail closed.
	core := f.p.core.(*fosite.Fosite)
	config := core.Config.(*fosite.Config)
	handlers := config.RevocationHandlers
	config.RevocationHandlers = nil
	revoke := func() int {
		form := url.Values{"client_id": {"test-client"}, "token": {first.RefreshToken}}
		r := httptest.NewRequest(http.MethodPost, f.p.Issuer()+"/oauth/revoke", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		f.p.Revoke(w, r)
		return w.Code
	}
	if status := revoke(); status != 503 {
		t.Fatalf("registered Fosite handler bypassed: %d", status)
	}
	if _, err := f.p.Authenticate(t.Context(), rotated.AccessToken, f.p.Resource("api")); err != nil {
		t.Fatal("failed revocation was committed", err)
	}
	config.RevocationHandlers = handlers
	if status := revoke(); status != 200 {
		t.Fatalf("Fosite revocation failed: %d", status)
	}
	if _, err := f.p.Authenticate(t.Context(), rotated.AccessToken, f.p.Resource("api")); err == nil {
		t.Fatal("consumed refresh did not revoke descendant family")
	}
}

func TestOAuthContinuationValidatesBrowserBindingPersistenceAndOutage(t *testing.T) {
	f := newProviderFixture(t)
	browser, _, err := newOpaqueToken("")
	if err != nil {
		t.Fatal(err)
	}
	handle, _, err := newOpaqueToken("")
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().UTC().Add(time.Minute).Truncate(time.Second)
	pending := pendingAuthorization{Client: f.options.Clients[0], RedirectURI: f.options.Clients[0].RedirectURIs[0], Resource: f.p.Resource("api"), Scopes: []string{ScopeSitesRead}, ExpiresAt: expires}
	payload, err := json.Marshal(pending)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.CreateOAuthPending(t.Context(), database.OAuthPending{Signature: tokenSignature(handle), BrowserSignature: tokenSignature(browser), Payload: payload, ExpiresAt: expires}); err != nil {
		t.Fatal(err)
	}
	request := func(browserValue, handleValue string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, f.p.Issuer()+"/api/oauth/continuation", nil)
		r.AddCookie(&http.Cookie{Name: browserCookieName, Value: browserValue})
		r.AddCookie(&http.Cookie{Name: pendingCookieName, Value: handleValue})
		return r
	}
	signed := f.p.signedHandle(handle, expires, browser)
	if pending, err := f.p.HasContinuation(t.Context(), request(browser, signed)); err != nil || !pending {
		t.Fatalf("valid continuation rejected: %v %v", pending, err)
	}
	for _, r := range []*http.Request{request(browser, "forged"), request(strings.Repeat("a", 43), signed), request(browser, f.p.signedHandle(strings.Repeat("a", 43), expires, browser)), request(browser, f.p.signedHandle(handle, time.Now().Add(-time.Minute), browser))} {
		if pending, err := f.p.HasContinuation(t.Context(), r); err != nil || pending {
			t.Fatalf("forged/stale continuation accepted: %v %v", pending, err)
		}
	}
	if err := f.store.Close(); err != nil {
		t.Fatal(err)
	}
	if pending, err := f.p.HasContinuation(t.Context(), request(browser, signed)); pending || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("storage outage treated as absent: %v %v", pending, err)
	}
}

// Hold the winning SQL transaction briefly so the other instance must handle a real conflict.
type delayedCommitProvider struct {
	fosite.OAuth2Provider
}

func (p delayedCommitProvider) NewAccessRequest(ctx context.Context, r *http.Request, s fosite.Session) (fosite.AccessRequester, error) {
	request, err := p.OAuth2Provider.NewAccessRequest(ctx, r, s)
	return request, err
}
func (p delayedCommitProvider) NewAccessResponse(ctx context.Context, r fosite.AccessRequester) (fosite.AccessResponder, error) {
	response, err := p.OAuth2Provider.NewAccessResponse(ctx, r)
	if err == nil {
		timer := time.NewTimer(25 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return response, err
}
