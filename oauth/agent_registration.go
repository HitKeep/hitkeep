package oauth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json/v2"
	"errors"
	"fmt"
	"math/big"
	"mime"
	"net/http"
	"net/mail"
	"net/url"
	"slices"
	"strings"
	"time"
	"uuid"

	googleuuid "github.com/google/uuid"

	"hitkeep/database"
)

const (
	agentClaimGrant    = "urn:workos:agent-auth:grant-type:claim"
	agentJWTGrant      = "urn:ietf:params:oauth:grant-type:jwt-bearer"
	agentClaimPrefix   = "hk_oauth_claim_"         // #nosec G101 -- Public credential-kind marker; body uses 32 crypto/rand bytes.
	agentAttemptPrefix = "hk_oauth_claim_attempt_" // #nosec G101 -- Public credential-kind marker; body uses 32 crypto/rand bytes.
	agentClientPrefix  = "urn:hitkeep:agent-registration:"
	agentCodeLifetime  = 10 * time.Minute
	agentPollInterval  = 5
	agentCodeAttempts  = 5
)

var ErrAgentClaimCode = errors.New("invalid agent claim code")

type agentRegistration struct {
	Type          string    `json:"type"`
	ID            uuid.UUID `json:"id"`
	Email         string    `json:"email"`
	Resource      string    `json:"resource"`
	Scopes        []string  `json:"scopes"`
	CodeSignature string    `json:"code_signature"`
	CodeExpiresAt time.Time `json:"code_expires_at"`
	NextPoll      time.Time `json:"next_poll"`
	Interval      int       `json:"interval"`
	Attempts      int       `json:"attempts"`
	GrantID       uuid.UUID `json:"grant_id"`
	Denied        bool      `json:"denied,omitzero"`
}

type agentClaimRef struct {
	ID               uuid.UUID `json:"id"`
	Signature        string    `json:"signature"`
	AttemptSignature string    `json:"attempt_signature"`
}

func (p *Provider) agentRegistrationEnabled() bool {
	return p.agentActionsEnabled != nil && p.agentActionsEnabled()
}

func (p *Provider) agentAuthMetadata() map[string]any {
	return map[string]any{
		"skill":                    p.urls.Issuer + "/auth.md",
		"identity_endpoint":        p.urls.Issuer + "/agent/identity",
		"claim_endpoint":           p.urls.Issuer + "/agent/identity/claim",
		"register_uri":             p.urls.Issuer + "/agent/identity",
		"claim_uri":                p.urls.Issuer + "/agent/identity/claim",
		"identity_types_supported": []string{"service_auth", "identity_assertion"},
		"identity_assertion": map[string]any{
			"assertion_types_supported":  []string{"verified_email"},
			"credential_types_supported": []string{"access_token"},
		},
	}
}

func decodeAgentJSON(w http.ResponseWriter, r *http.Request, value any) error {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
		return ErrInvalidConsent
	}
	return json.UnmarshalRead(http.MaxBytesReader(w, r.Body, 16<<10), value, json.RejectUnknownMembers(true))
}

func agentEmail(raw string) (string, error) {
	if len(raw) > 254 || raw != strings.TrimSpace(raw) {
		return "", ErrInvalidConsent
	}
	parsed, err := mail.ParseAddress(raw)
	if err != nil || parsed.Address != raw {
		return "", ErrInvalidConsent
	}
	return strings.ToLower(raw), nil
}

func (p *Provider) agentScopes(resource, raw string) ([]string, error) {
	allowed := scopesForResource(p, resource)
	if raw == "" {
		raw = ScopeSitesRead + " " + ScopeAnalyticsRead
	}
	scopes := strings.Fields(raw)
	if len(allowed) == 0 || len(scopes) == 0 || len(scopes) > len(allowed) {
		return nil, ErrInvalidConsent
	}
	seen := make(map[string]bool, len(scopes))
	for _, scope := range scopes {
		if !slices.Contains(allowed, scope) || seen[scope] {
			return nil, ErrInvalidConsent
		}
		seen[scope] = true
	}
	if resource == p.urls.MCPResource && (!seen[ScopeSitesRead] || !seen[ScopeAnalyticsRead]) {
		return nil, ErrInvalidConsent
	}
	return scopes, nil
}

func (p *Provider) newAgentAttempt(state *agentRegistration, pending *database.OAuthPending) (string, string, error) {
	attempt, signature, err := newOpaqueToken(agentAttemptPrefix)
	if err != nil {
		return "", "", err
	}
	number, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", "", err
	}
	code := fmt.Sprintf("%06d", number.Int64())
	state.CodeSignature = tokenSignature(state.ID.String() + "\x00" + code)
	state.CodeExpiresAt = minTime(time.Now().UTC().Add(agentCodeLifetime), pending.ExpiresAt)
	state.NextPoll = time.Time{}
	state.Interval = agentPollInterval
	pending.BrowserSignature = signature
	return attempt, code, nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (p *Provider) agentCeremony(state *agentRegistration, attempt, code string) map[string]any {
	return map[string]any{
		"user_code":        code,
		"expires_in":       max(0, int(time.Until(state.CodeExpiresAt).Seconds())),
		"verification_uri": p.urls.Issuer + "/oauth/agent-claim?claim_attempt_token=" + url.QueryEscape(attempt),
		"interval":         state.Interval,
	}
}

func (p *Provider) AgentIdentity(w http.ResponseWriter, r *http.Request) {
	publicOAuthCORS(w)
	if !p.agentRegistrationEnabled() {
		http.NotFound(w, r)
		return
	}
	if !p.Ready() {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
		return
	}
	var request struct {
		Type          string `json:"type"`
		LoginHint     string `json:"login_hint"`
		Resource      string `json:"resource"`
		Scope         string `json:"scope"`
		AssertionType string `json:"assertion_type"`
		Assertion     string `json:"assertion"`
	}
	if decodeAgentJSON(w, r, &request) != nil {
		oauthError(w, 400, "invalid_request")
		return
	}
	registrationType, status := "service_auth", http.StatusCreated
	// Auth.md v0.5's email shape is the same human-claim ceremony, never a provider proof.
	if request.Type == "identity_assertion" && request.AssertionType == "verified_email" && request.LoginHint == "" {
		request.Type, request.LoginHint = "service_auth", request.Assertion
		registrationType, status = "email-verification", http.StatusOK
	} else if request.AssertionType != "" || request.Assertion != "" {
		oauthError(w, 400, "invalid_request")
		return
	}
	if request.Type != "service_auth" {
		code := "invalid_request"
		if request.Type == "anonymous" || request.Type == "identity_assertion" {
			code = request.Type + "_not_enabled"
		}
		oauthError(w, 400, code)
		return
	}
	email, err := agentEmail(request.LoginHint)
	if err != nil {
		oauthError(w, 400, "invalid_request")
		return
	}
	resource := request.Resource
	if resource == "" {
		resource = p.urls.APIResource
	}
	scopes, err := p.agentScopes(resource, request.Scope)
	if err != nil {
		oauthError(w, 400, "invalid_scope")
		return
	}
	claim, signature, err := newOpaqueToken(agentClaimPrefix)
	if err != nil {
		oauthError(w, 503, "temporarily_unavailable")
		return
	}
	pending := database.OAuthPending{Signature: signature, ExpiresAt: time.Now().UTC().Add(browserLifetime)}
	state := agentRegistration{Type: "service_auth", ID: uuid.NewV7(), Email: email, Resource: resource, Scopes: scopes}
	attempt, code, err := p.newAgentAttempt(&state, &pending)
	if err == nil {
		pending.Payload, err = json.Marshal(state)
	}
	if err != nil {
		oauthError(w, 503, "temporarily_unavailable")
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	tx, err := p.store.BeginOAuth(r.Context())
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		err = tx.CreatePending(r.Context(), pending)
		if err == nil {
			err = tx.AppendAudit(r.Context(), uuid.Nil(), uuid.Nil(), state.ID, "oauth.agent_registration_created")
		}
		if err == nil && (!p.Ready() || !p.agentRegistrationEnabled()) {
			err = ErrUnavailable
		}
		if err == nil {
			err = tx.Commit()
		}
	}
	if err != nil {
		if errors.Is(err, database.ErrOAuthCapacity) {
			oauthError(w, 429, "temporarily_unavailable")
		} else {
			oauthError(w, 503, "temporarily_unavailable")
		}
		return
	}
	if !p.Ready() || !p.agentRegistrationEnabled() {
		oauthError(w, 503, "temporarily_unavailable")
		return
	}
	writeJSON(w, status, map[string]any{"registration_id": state.ID.String(), "registration_type": registrationType, "claim_url": p.urls.Issuer + "/agent/identity/claim", "claim_token": claim, "claim_token_expires": pending.ExpiresAt, "post_claim_scopes": scopes, "claim": p.agentCeremony(&state, attempt, code)})
}

func (p *Provider) loadAgentPending(ctx context.Context, tx *database.OAuthTransaction, claim, attempt string) (*agentRegistration, *database.OAuthPending, error) {
	pending, err := tx.GetAgentPending(ctx, claim, attempt)
	if err != nil {
		return nil, nil, err
	}
	var state agentRegistration
	if err := json.Unmarshal(pending.Payload, &state); err != nil || state.Type != "service_auth" || state.ID == uuid.Nil() || state.Email == "" || state.Interval < agentPollInterval || state.Attempts >= agentCodeAttempts {
		return nil, nil, ErrInvalidConsent
	}
	if scopes, err := p.agentScopes(state.Resource, strings.Join(state.Scopes, " ")); err != nil || !slices.Equal(scopes, state.Scopes) {
		return nil, nil, ErrInvalidConsent
	}
	return &state, pending, nil
}

func (p *Provider) AgentClaim(w http.ResponseWriter, r *http.Request) {
	publicOAuthCORS(w)
	if !p.agentRegistrationEnabled() {
		http.NotFound(w, r)
		return
	}
	if !p.Ready() {
		oauthError(w, 503, "temporarily_unavailable")
		return
	}
	var request struct {
		ClaimToken string `json:"claim_token"`
		Email      string `json:"email"`
	}
	if decodeAgentJSON(w, r, &request) != nil || !validOpaqueToken(request.ClaimToken, agentClaimPrefix) {
		oauthError(w, 400, "invalid_claim_token")
		return
	}
	email, err := agentEmail(request.Email)
	if err != nil {
		oauthError(w, 400, "invalid_claim_token")
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	tx, err := p.store.BeginOAuth(r.Context())
	if err != nil {
		oauthError(w, 503, "temporarily_unavailable")
		return
	}
	defer func() { _ = tx.Rollback() }()
	state, pending, err := p.loadAgentPending(r.Context(), tx, tokenSignature(request.ClaimToken), "")
	if err != nil {
		if errors.Is(err, database.ErrOAuthNotFound) || errors.Is(err, ErrInvalidConsent) {
			oauthError(w, 400, "invalid_claim_token")
		} else {
			oauthError(w, 503, "temporarily_unavailable")
		}
		return
	}
	if state.Email != email {
		oauthError(w, 400, "invalid_claim_token")
		return
	}
	if state.GrantID != uuid.Nil() || state.Denied {
		oauthError(w, 400, "claimed_or_in_flight")
		return
	}
	attempt, code, err := p.newAgentAttempt(state, pending)
	if err == nil {
		pending.Payload, err = json.Marshal(state)
	}
	if err == nil {
		err = tx.UpdateAgentPending(r.Context(), *pending)
	}
	if err == nil {
		err = tx.AppendAudit(r.Context(), uuid.Nil(), uuid.Nil(), state.ID, "oauth.agent_claim_started")
	}
	if err == nil && (!p.Ready() || !p.agentRegistrationEnabled()) {
		err = ErrUnavailable
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		oauthError(w, 503, "temporarily_unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"registration_id": state.ID.String(), "status": "initiated", "expires_at": state.CodeExpiresAt, "claim_attempt": p.agentCeremony(state, attempt, code)})
}

func (p *Provider) StartAgentClaim(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if !p.agentRegistrationEnabled() {
		http.NotFound(w, r)
		return
	}
	if !p.Ready() {
		oauthError(w, 503, "temporarily_unavailable")
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) != 1 || len(query["claim_attempt_token"]) != 1 || !validOpaqueToken(query.Get("claim_attempt_token"), agentAttemptPrefix) {
		oauthError(w, 400, "invalid_claim_token")
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	tx, err := p.store.BeginOAuth(r.Context())
	if err != nil {
		oauthError(w, 503, "temporarily_unavailable")
		return
	}
	state, record, err := p.loadAgentPending(r.Context(), tx, "", tokenSignature(query.Get("claim_attempt_token")))
	_ = tx.Rollback()
	if err != nil {
		if errors.Is(err, database.ErrOAuthNotFound) || errors.Is(err, ErrInvalidConsent) {
			oauthError(w, 400, "invalid_claim_token")
		} else {
			oauthError(w, 503, "temporarily_unavailable")
		}
		return
	}
	if state.GrantID != uuid.Nil() || state.Denied || !time.Now().UTC().Before(state.CodeExpiresAt) {
		oauthError(w, 400, "invalid_claim_token")
		return
	}
	browser := ""
	if cookie, err := r.Cookie(browserCookieName); err == nil && validOpaqueToken(cookie.Value, "") {
		browser = cookie.Value
	}
	if browser == "" {
		browser, _, err = newOpaqueToken("")
	}
	if err != nil {
		oauthError(w, 503, "temporarily_unavailable")
		return
	}
	handle, _, err := newOpaqueToken("")
	if err != nil {
		oauthError(w, 503, "temporarily_unavailable")
		return
	}
	redirect := p.urls.Issuer + "/oauth/consent?claim=complete"
	client := ClientMetadata{ID: agentClientPrefix + state.ID.String(), Name: "HitKeep Agent", URI: p.urls.Issuer, RedirectURIs: []string{redirect}, AuthMethod: "none"}
	pending := pendingAuthorization{Client: client, RedirectURI: redirect, Resource: state.Resource, Scopes: state.Scopes, ExpiresAt: minTime(record.ExpiresAt, state.CodeExpiresAt), AgentClaim: &agentClaimRef{ID: state.ID, Signature: record.Signature, AttemptSignature: record.BrowserSignature}}
	payload, err := json.Marshal(pending)
	if err == nil {
		err = p.store.CreateOAuthPending(r.Context(), database.OAuthPending{Signature: tokenSignature(handle), BrowserSignature: tokenSignature(browser), Payload: payload, ExpiresAt: pending.ExpiresAt})
	}
	if err != nil {
		oauthError(w, 503, "temporarily_unavailable")
		return
	}
	p.setBrowserCookie(w, browserCookieName, browser, int(time.Until(pending.ExpiresAt).Seconds()))
	p.setBrowserCookie(w, pendingCookieName, p.signedHandle(handle, pending.ExpiresAt, browser), int(time.Until(pending.ExpiresAt).Seconds()))
	http.Redirect(w, r, p.urls.Issuer+"/oauth/consent", http.StatusSeeOther)
}

func (p *Provider) agentClaimForUser(ctx context.Context, tx *database.OAuthTransaction, pending *pendingAuthorization, userID uuid.UUID) (*agentRegistration, *database.OAuthPending, error) {
	if pending.AgentClaim == nil || !p.agentRegistrationEnabled() {
		return nil, nil, ErrInvalidConsent
	}
	state, record, err := p.loadAgentPending(ctx, tx, pending.AgentClaim.Signature, "")
	if err != nil {
		if errors.Is(err, database.ErrOAuthNotFound) || errors.Is(err, ErrInvalidConsent) {
			return nil, nil, ErrInvalidConsent
		}
		return nil, nil, ErrUnavailable
	}
	if state.ID != pending.AgentClaim.ID || state.GrantID != uuid.Nil() || state.Denied || !time.Now().UTC().Before(state.CodeExpiresAt) || subtle.ConstantTimeCompare([]byte(record.BrowserSignature), []byte(pending.AgentClaim.AttemptSignature)) != 1 || state.Resource != pending.Resource || !slices.Equal(state.Scopes, pending.Scopes) {
		return nil, nil, ErrInvalidConsent
	}
	user, err := p.store.GetUserByID(ctx, googleuuid.UUID(userID))
	if err != nil {
		return nil, nil, ErrUnavailable
	}
	if user == nil || !strings.EqualFold(user.Email, state.Email) {
		return nil, nil, ErrConsentForbidden
	}
	return state, record, nil
}
