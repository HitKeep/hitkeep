package oauth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json/v2"
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ory/fosite"
	fositeoauth2 "github.com/ory/fosite/handler/oauth2"
	"github.com/ory/fosite/handler/pkce"

	"hitkeep/auth"
	"hitkeep/database"
)

const (
	ScopeSitesRead           = "sites:read"
	ScopeAnalyticsRead       = "analytics:read"
	ScopeTrackingDomainsRead = "tracking_domains:read"
	ScopeIngestWrite         = "ingest:write"
	codeLifetime             = time.Minute
	accessLifetime           = 5 * time.Minute
	refreshLifetime          = 30 * 24 * time.Hour
	grantLifetime            = 90 * 24 * time.Hour
	browserLifetime          = 15 * time.Minute
)

var ErrUnavailable = errors.New("OAuth authoritative control plane unavailable")
var ErrInvalidToken = errors.New("invalid OAuth credential")
var ErrInvalidConsent = errors.New("invalid OAuth consent")
var ErrConsentForbidden = errors.New("OAuth consent forbidden")

type Options struct {
	Issuer            string
	MCPPath           string
	MCPEnabled        bool
	BrowserSigningKey []byte
	Clients           []ClientMetadata
	IsLeader          func() bool
}
type Provider struct {
	store      *database.Store
	urls       issuerURLs
	clients    *clientResolver
	browserKey []byte
	isLeader   func() bool
	mcpEnabled bool
	core       fosite.OAuth2Provider
	storage    *fositeStorage
	strategy   opaqueStrategy
	mu         sync.Mutex
}

func New(store *database.Store, options Options) (*Provider, error) {
	if store == nil || len(options.BrowserSigningKey) < 32 || options.IsLeader == nil {
		return nil, errors.New("OAuth requires a store, browser signing key and authoritative role")
	}
	urls, err := parseIssuer(options.Issuer, options.MCPPath)
	if err != nil {
		return nil, err
	}
	clients, err := newClientResolver(options.Clients)
	if err != nil {
		return nil, err
	}
	key := hmac.New(sha256.New, options.BrowserSigningKey)
	key.Write([]byte("hitkeep/oauth/browser/v1\x00" + urls.Issuer))
	p := &Provider{mcpEnabled: options.MCPEnabled, store: store, urls: urls, clients: clients, browserKey: key.Sum(nil), isLeader: options.IsLeader, strategy: opaqueStrategy{}}
	storage := &fositeStorage{provider: p}
	p.storage = storage
	config := &fosite.Config{AuthorizeCodeLifespan: codeLifetime, AccessTokenLifespan: accessLifetime, RefreshTokenLifespan: refreshLifetime, EnforcePKCE: true, EnforcePKCEForPublicClients: true, EnablePKCEPlainChallengeMethod: false, ScopeStrategy: fosite.ExactScopeStrategy, AudienceMatchingStrategy: fosite.ExactAudienceMatchingStrategy, RefreshTokenScopes: []string{}, SendDebugMessagesToClients: false, TokenURL: urls.Issuer + "/oauth/token"}
	config.ClientAuthenticationStrategy = func(ctx context.Context, r *http.Request, form url.Values) (fosite.Client, error) {
		if r.Header.Get("Authorization") != "" || form.Has("client_secret") || form.Has("client_assertion") || form.Has("client_assertion_type") {
			return nil, fosite.ErrInvalidClient
		}
		return storage.GetClient(ctx, form.Get("client_id"))
	}
	explicit := &fositeoauth2.AuthorizeExplicitGrantHandler{AccessTokenStrategy: p.strategy, RefreshTokenStrategy: p.strategy, AuthorizeCodeStrategy: p.strategy, CoreStorage: storage, TokenRevocationStorage: storage, Config: config}
	refresh := &fositeoauth2.RefreshTokenGrantHandler{AccessTokenStrategy: p.strategy, RefreshTokenStrategy: p.strategy, TokenRevocationStorage: storage, Config: config}
	proof := &pkce.Handler{AuthorizeCodeStrategy: p.strategy, Storage: storage, Config: config}
	config.AuthorizeEndpointHandlers.Append(explicit)
	config.AuthorizeEndpointHandlers.Append(proof)
	config.TokenEndpointHandlers.Append(explicit)
	config.TokenEndpointHandlers.Append(refresh)
	config.TokenEndpointHandlers.Append(proof)
	config.RevocationHandlers.Append(&fositeoauth2.TokenRevocationHandler{TokenRevocationStorage: storage, AccessTokenStrategy: p.strategy, RefreshTokenStrategy: p.strategy})
	p.core = fosite.NewOAuth2Provider(storage, config)
	return p, nil
}
func (p *Provider) Issuer() string { return p.urls.Issuer }
func (p *Provider) Resource(kind string) string {
	switch kind {
	case "api":
		return p.urls.APIResource
	case "mcp":
		if !p.mcpEnabled {
			return ""
		}
		return p.urls.MCPResource
	default:
		return ""
	}
}
func (p *Provider) DiscoveryPaths() []string {
	paths := []string{p.urls.AuthorizationMetadataPath, p.urls.APIResourceMetadataPath}
	if p.mcpEnabled {
		paths = append(paths, p.urls.MCPResourceMetadataPath)
	}
	return paths
}
func (p *Provider) Ready() bool { return p != nil && p.isLeader != nil && p.isLeader() }
func scopesForResource(p *Provider, resource string) []string {
	if resource == p.urls.APIResource {
		return []string{ScopeSitesRead, ScopeAnalyticsRead, ScopeTrackingDomainsRead, ScopeIngestWrite}
	}
	if p.mcpEnabled && resource == p.urls.MCPResource {
		return []string{ScopeSitesRead, ScopeAnalyticsRead}
	}
	return nil
}
func (p *Provider) fositeClient(m ClientMetadata) fosite.Client {
	grants := []string{"authorization_code", "refresh_token"}
	if len(m.GrantTypes) > 0 && !slices.Contains(m.GrantTypes, "refresh_token") {
		grants = []string{"authorization_code"}
	}
	return &fosite.DefaultClient{ID: m.ID, RedirectURIs: slices.Clone(m.RedirectURIs), GrantTypes: grants, ResponseTypes: []string{"code"}, Scopes: scopesForResource(p, p.urls.APIResource), Audience: []string{p.urls.APIResource, p.urls.MCPResource}, Public: true}
}
func (p *Provider) Metadata(w http.ResponseWriter, r *http.Request) {
	publicOAuthCORS(w)
	writeJSON(w, http.StatusOK, map[string]any{"issuer": p.urls.Issuer, "authorization_endpoint": p.urls.Issuer + "/oauth/authorize", "token_endpoint": p.urls.Issuer + "/oauth/token", "revocation_endpoint": p.urls.Issuer + "/oauth/revoke", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"}, "code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"}, "revocation_endpoint_auth_methods_supported": []string{"none"}, "client_id_metadata_document_supported": true, "authorization_response_iss_parameter_supported": true, "scopes_supported": scopesForResource(p, p.urls.APIResource)})
}
func (p *Provider) ProtectedResourceMetadata(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		publicOAuthCORS(w)
		resource := p.Resource(kind)
		if resource == "" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"resource": resource, "authorization_servers": []string{p.urls.Issuer}, "bearer_methods_supported": []string{"header"}, "scopes_supported": scopesForResource(p, resource)})
	}
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.MarshalWrite(w, value)
}
func oauthError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}
func parseTokenForm(w http.ResponseWriter, r *http.Request) (url.Values, error) {
	content, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || content != "application/x-www-form-urlencoded" || r.URL.RawQuery != "" {
		return nil, fosite.ErrInvalidRequest
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		return nil, fosite.ErrInvalidRequest
	}
	for _, values := range r.PostForm {
		if len(values) != 1 || len(values[0]) > 4096 {
			return nil, fosite.ErrInvalidRequest
		}
	}
	if r.Header.Get("Authorization") != "" || r.PostForm.Has("client_secret") || r.PostForm.Has("client_assertion") || r.PostForm.Has("client_assertion_type") || r.PostForm.Has("audience") {
		return nil, fosite.ErrInvalidClient
	}
	return r.PostForm.Clone(), nil
}
func (p *Provider) Token(w http.ResponseWriter, r *http.Request) {
	publicOAuthCORS(w)
	if !p.Ready() {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
		return
	}
	form, err := parseTokenForm(w, r)
	if err != nil {
		p.core.WriteAccessError(r.Context(), w, nil, err)
		return
	}
	kind, raw := "code", form.Get("code")
	if form.Get("grant_type") == "refresh_token" {
		kind = "refresh"
		raw = form.Get("refresh_token")
	} else if form.Get("grant_type") != "authorization_code" {
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type")
		return
	}
	prefix := authorizeCodePrefix
	if kind == "refresh" {
		prefix = refreshTokenPrefix
	}
	if !validOpaqueToken(raw, prefix) || form.Get("client_id") == "" || len(scopesForResource(p, form.Get("resource"))) == 0 {
		oauthError(w, http.StatusBadRequest, "invalid_grant")
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var responder fosite.AccessResponder
	var requester fosite.AccessRequester
	operationContext, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), metadataTimeout)
	defer cancel()
	for attempt := 0; ; attempt++ {
		ctx := operationContext
		tx, beginErr := p.store.BeginOAuth(ctx)
		if beginErr != nil {
			oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
			return
		}
		credential, loadErr := tx.GetCredential(ctx, kind, tokenSignature(raw))
		if loadErr != nil && !errors.Is(loadErr, database.ErrOAuthNotFound) {
			_ = tx.Rollback()
			oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
			return
		}
		var snapshot storedRequest
		if loadErr == nil {
			if err := json.Unmarshal(credential.Payload, &snapshot); err != nil {
				_ = tx.Rollback()
				oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
				return
			}
		}
		if loadErr != nil || snapshot.Client.ID != form.Get("client_id") || snapshot.Audience != form.Get("resource") {
			_ = tx.Rollback()
			oauthError(w, http.StatusBadRequest, "invalid_grant")
			return
		}
		if requested := form.Get("scope"); requested != "" {
			if strings.Join(strings.Fields(requested), " ") != strings.Join(snapshot.GrantedScopes, " ") {
				_ = tx.Rollback()
				oauthError(w, http.StatusBadRequest, "invalid_scope")
				return
			}
		}
		op := &providerOperation{tx: tx, client: snapshot.Client, resource: snapshot.Audience}
		ctx = context.WithValue(ctx, operationKey{}, op)
		request := r.Clone(ctx)
		request.Form = form.Clone()
		request.PostForm = form.Clone()
		request.PostForm.Set("audience", snapshot.Audience)
		request.Form.Set("audience", snapshot.Audience)
		requester, err = p.core.NewAccessRequest(ctx, request, &oauthSession{})
		if err == nil {
			responder, err = p.core.NewAccessResponse(ctx, requester)
		}
		if err == nil && (!p.Ready() || op.rollback) {
			err = fosite.ErrTemporarilyUnavailable
		}
		commit := err == nil || (op.securityRevocation && !op.rollback && errors.Is(err, fosite.ErrInvalidGrant))
		if commit {
			if commitErr := tx.Commit(); commitErr != nil {
				err = commitErr
			} else if op.securityRevocation {
				if grant, loadErr := p.store.GetOAuthGrant(ctx, snapshot.Session.GrantID); loadErr == nil {
					if auditErr := p.store.AppendOAuthReuseAudit(ctx, grant.UserID, grant.TeamID, grant.ID, kind == "refresh"); auditErr != nil {
						slog.Warn("Could not record OAuth reuse audit")
					}
				} else {
					slog.Warn("Could not record OAuth reuse audit")
				}
			}
		} else {
			_ = tx.Rollback()
		}
		if errors.Is(err, database.ErrOAuthConflict) || errors.Is(err, fosite.ErrSerializationFailure) {
			_ = tx.Rollback()
			// Let the competing transaction commit before reading its replay tombstone.
			timer := time.NewTimer(min(10*time.Millisecond<<min(attempt, 4), 100*time.Millisecond))
			select {
			case <-operationContext.Done():
				timer.Stop()
				oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
				return
			case <-timer.C:
				continue
			}
		}
		break
	}
	if err != nil {
		if operationContext.Err() != nil || errors.Is(err, database.ErrOAuthConflict) || errors.Is(err, fosite.ErrSerializationFailure) || errors.Is(err, context.DeadlineExceeded) {
			oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
			return
		}
		p.core.WriteAccessError(r.Context(), w, requester, err)
		return
	}
	responder.SetExtra("resource", form.Get("resource"))
	p.core.WriteAccessResponse(r.Context(), w, requester, responder)
}

func (p *Provider) Authenticate(ctx context.Context, token, audience string) (*auth.OAuthGrant, error) {
	if !IsOAuthToken(token) {
		return nil, nil
	}
	if !p.Ready() {
		return nil, ErrUnavailable
	}
	if !validOpaqueToken(token, accessTokenPrefix) || len(scopesForResource(p, audience)) == 0 {
		return nil, ErrInvalidToken
	}
	tx, err := p.store.BeginOAuth(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer func() { _ = tx.Rollback() }()
	c, err := tx.GetCredential(ctx, "access", tokenSignature(token))
	if err != nil {
		if errors.Is(err, database.ErrOAuthNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, ErrUnavailable
	}
	if c.Consumed || c.Revoked || !time.Now().UTC().Before(c.ExpiresAt) {
		return nil, ErrInvalidToken
	}
	grant, err := tx.GetGrant(ctx, c.GrantID)
	if err != nil {
		if errors.Is(err, database.ErrOAuthNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, ErrUnavailable
	}
	if !grant.Active || grant.Audience != audience {
		return nil, ErrInvalidToken
	}
	if !p.Ready() {
		return nil, ErrUnavailable
	}
	return &auth.OAuthGrant{ID: grant.ID, UserID: grant.UserID, TeamID: grant.TeamID, SiteIDs: slices.Clone(grant.SiteIDs), Scopes: slices.Clone(grant.Scopes), Audience: grant.Audience}, nil
}
func (p *Provider) Revoke(w http.ResponseWriter, r *http.Request) {
	publicOAuthCORS(w)
	if !p.Ready() {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
		return
	}
	form, err := parseTokenForm(w, r)
	if err != nil {
		p.core.WriteRevocationResponse(r.Context(), w, err)
		return
	}
	raw := form.Get("token")
	kind := "access"
	prefix := accessTokenPrefix
	if strings.HasPrefix(raw, refreshTokenPrefix) {
		kind = "refresh"
		prefix = refreshTokenPrefix
	}
	if !validOpaqueToken(raw, prefix) || form.Get("client_id") == "" {
		p.core.WriteRevocationResponse(r.Context(), w, nil)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), metadataTimeout)
	defer cancel()
	tx, err := p.store.BeginOAuth(ctx)
	if err != nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
		return
	}
	defer func() { _ = tx.Rollback() }()
	c, err := tx.GetCredential(ctx, kind, tokenSignature(raw))
	if errors.Is(err, database.ErrOAuthNotFound) {
		p.core.WriteRevocationResponse(ctx, w, nil)
		return
	}
	if err != nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
		return
	}
	grant, err := tx.GetGrant(ctx, c.GrantID)
	if err != nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
		return
	}
	if grant.ClientID != form.Get("client_id") || (form.Get("resource") != "" && grant.Audience != form.Get("resource")) {
		p.core.WriteRevocationResponse(ctx, w, nil)
		return
	}
	var snapshot storedRequest
	if err := json.Unmarshal(c.Payload, &snapshot); err != nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
		return
	}
	op := &providerOperation{tx: tx, client: snapshot.Client, resource: grant.Audience, revoking: true}
	ctx = context.WithValue(ctx, operationKey{}, op)
	request := r.Clone(ctx)
	request.Form = form.Clone()
	request.PostForm = form.Clone()
	if err := p.core.NewRevocationRequest(ctx, request); err != nil || op.rollback {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
		return
	}
	if !p.Ready() {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
		return
	}
	if err := tx.AppendAudit(ctx, grant.UserID, grant.TeamID, grant.ID, "oauth.grant_revoked"); err != nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
		return
	}
	if err := tx.Commit(); err != nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
		return
	}
	p.core.WriteRevocationResponse(ctx, w, nil)
}

func (p *Provider) ResourceMetadataURL(kind string) string {
	if p.Resource(kind) == "" {
		return ""
	}
	target, err := url.Parse(p.urls.Issuer)
	if err != nil {
		return ""
	}
	paths := p.DiscoveryPaths()
	switch kind {
	case "api":
		target.Path = paths[1]
	case "mcp":
		target.Path = paths[2]
	default:
		return ""
	}
	target.RawPath = ""
	return target.String()
}
func (p *Provider) Preflight(w http.ResponseWriter, r *http.Request) {
	publicOAuthCORS(w)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Max-Age", "300")
	w.WriteHeader(http.StatusNoContent)
}
func publicOAuthCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Del("Access-Control-Allow-Credentials")
}
