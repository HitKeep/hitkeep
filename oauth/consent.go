package oauth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/ory/fosite"

	"hitkeep/api"
	"hitkeep/auth"
	"hitkeep/database"
)

const pendingCookieName = "hk_oauth_pending"
const browserCookieName = "hk_oauth_browser"

type pendingAuthorization struct {
	Client      ClientMetadata `json:"client"`
	RedirectURI string         `json:"redirect_uri"`
	Resource    string         `json:"resource"`
	Scopes      []string       `json:"scopes"`
	State       string         `json:"state"`
	Challenge   string         `json:"code_challenge"`
	ExpiresAt   time.Time      `json:"expires_at"`
}

func (p *Provider) Authorize(w http.ResponseWriter, r *http.Request) {
	if !p.Ready() {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
		return
	}
	if len(r.URL.RawQuery) > 8<<10 {
		oauthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	form, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	for _, values := range form {
		if len(values) != 1 || len(values[0]) > 2048 {
			oauthError(w, http.StatusBadRequest, "invalid_request")
			return
		}
	}
	client, err := p.resolveClient(r.Context(), form.Get("client_id"))
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client")
		return
	}
	redirect := form.Get("redirect_uri")
	if !form.Has("redirect_uri") && len(client.RedirectURIs) == 1 {
		redirect = client.RedirectURIs[0]
		form.Set("redirect_uri", redirect)
	}
	if !redirectAllowed(client, redirect) {
		oauthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if strings.HasPrefix(redirect, "http://") {
		// Fosite only varies IP-loopback ports; pin our validated native callback for consent and code binding.
		client.RedirectURIs = []string{redirect}
	}
	state := form.Get("state")
	fail := func(code string) { p.callback(w, r, redirect, state, "", code) }
	resource := form.Get("resource")
	scopes := strings.Fields(form.Get("scope"))
	allowed := scopesForResource(p, resource)
	if len(allowed) == 0 {
		fail("invalid_target")
		return
	}
	if form.Get("response_type") != "code" || form.Get("response_mode") != "" || form.Has("audience") || form.Has("request") || form.Has("request_uri") || (form.Has("state") && state == "") || len(state) > 1024 || form.Get("code_challenge_method") != "S256" {
		fail("invalid_request")
		return
	}
	challenge, err := base64.RawURLEncoding.DecodeString(form.Get("code_challenge"))
	if err != nil || len(challenge) != 32 || len(form.Get("code_challenge")) != 43 {
		fail("invalid_request")
		return
	}
	if len(scopes) == 0 || len(scopes) > 4 {
		fail("invalid_scope")
		return
	}
	seen := make(map[string]bool, len(scopes))
	for _, scope := range scopes {
		if !slices.Contains(allowed, scope) || seen[scope] {
			fail("invalid_scope")
			return
		}
		seen[scope] = true
	}
	if resource == p.urls.MCPResource && (!seen[ScopeSitesRead] || !seen[ScopeAnalyticsRead]) {
		fail("invalid_scope")
		return
	}
	// Validate with Fosite against the same pinned client before creating browser state.
	tx, err := p.store.BeginOAuth(r.Context())
	if err != nil {
		fail("temporarily_unavailable")
		return
	}
	op := &providerOperation{tx: tx, client: client, resource: resource}
	ctx := context.WithValue(r.Context(), operationKey{}, op)
	validated := r.Clone(ctx)
	validated.URL = r.URL.Clone()
	form.Set("audience", resource)
	validated.URL.RawQuery = form.Encode()
	validated.Form = form.Clone()
	_, err = p.newAuthorizeRequest(ctx, validated)
	_ = tx.Rollback()
	if err != nil {
		fail(fosite.ErrorToRFC6749Error(err).ErrorField)
		return
	}
	browser := ""
	if cookie, err := r.Cookie(browserCookieName); err == nil && validOpaqueToken(cookie.Value, "") {
		browser = cookie.Value
	}
	if browser == "" {
		browser, _, err = newOpaqueToken("")
		if err != nil {
			fail("temporarily_unavailable")
			return
		}
	}
	handle, _, err := newOpaqueToken("")
	if err != nil {
		fail("temporarily_unavailable")
		return
	}
	pending := pendingAuthorization{Client: client, RedirectURI: redirect, Resource: resource, Scopes: scopes, State: state, Challenge: form.Get("code_challenge"), ExpiresAt: time.Now().UTC().Add(browserLifetime)}
	payload, err := json.Marshal(pending)
	if err != nil {
		fail("temporarily_unavailable")
		return
	}
	p.mu.Lock()
	err = p.store.CreateOAuthPending(r.Context(), database.OAuthPending{Signature: tokenSignature(handle), BrowserSignature: tokenSignature(browser), Payload: payload, ExpiresAt: pending.ExpiresAt})
	p.mu.Unlock()
	if err != nil {
		fail("temporarily_unavailable")
		return
	}
	p.setBrowserCookie(w, browserCookieName, browser, int(browserLifetime.Seconds()))
	p.setBrowserCookie(w, pendingCookieName, p.signedHandle(handle, pending.ExpiresAt, browser), int(browserLifetime.Seconds()))
	http.Redirect(w, r, p.urls.Issuer+"/oauth/consent", http.StatusSeeOther)
}
func (p *Provider) signedHandle(handle string, expires time.Time, browser string) string {
	value := handle + "." + strconv.FormatInt(expires.Unix(), 10)
	mac := hmac.New(sha256.New, p.browserKey)
	mac.Write([]byte("pending\x00" + value + "\x00" + browser))
	return value + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (p *Provider) setBrowserCookie(w http.ResponseWriter, name, value string, maxAge int) {
	cookiePath := p.urls.Prefix
	if cookiePath == "" {
		cookiePath = "/"
	}
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: cookiePath, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}
func (p *Provider) pending(ctx context.Context, r *http.Request) (*pendingAuthorization, string, string, error) {
	if !p.Ready() {
		return nil, "", "", ErrUnavailable
	}
	signed, err := r.Cookie(pendingCookieName)
	if err != nil {
		return nil, "", "", ErrInvalidConsent
	}
	browser, err := r.Cookie(browserCookieName)
	if err != nil || !validOpaqueToken(browser.Value, "") {
		return nil, "", "", ErrInvalidConsent
	}
	pieces := strings.Split(signed.Value, ".")
	if len(pieces) != 3 || !validOpaqueToken(pieces[0], "") {
		return nil, "", "", ErrInvalidConsent
	}
	seconds, err := strconv.ParseInt(pieces[1], 10, 64)
	if err != nil || !time.Now().UTC().Before(time.Unix(seconds, 0)) {
		return nil, "", "", ErrInvalidConsent
	}
	expected := p.signedHandle(pieces[0], time.Unix(seconds, 0), browser.Value)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(signed.Value)) != 1 {
		return nil, "", "", ErrInvalidConsent
	}
	record, err := p.store.GetOAuthPending(ctx, tokenSignature(pieces[0]), tokenSignature(browser.Value))
	if err != nil {
		if errors.Is(err, database.ErrOAuthNotFound) {
			return nil, "", "", ErrInvalidConsent
		}
		return nil, "", "", ErrUnavailable
	}
	var pending pendingAuthorization
	if err := json.Unmarshal(record.Payload, &pending); err != nil || pending.ExpiresAt.Unix() != seconds || !time.Now().UTC().Before(pending.ExpiresAt) || !redirectAllowed(pending.Client, pending.RedirectURI) {
		return nil, "", "", ErrInvalidConsent
	}
	allowed := scopesForResource(p, pending.Resource)
	if len(allowed) == 0 || len(pending.Scopes) == 0 {
		return nil, "", "", ErrInvalidConsent
	}
	for _, scope := range pending.Scopes {
		if !slices.Contains(allowed, scope) {
			return nil, "", "", ErrInvalidConsent
		}
	}
	return &pending, record.Signature, record.BrowserSignature, nil
}

// Continuation exposes only validated local navigation state.
func (p *Provider) HasContinuation(ctx context.Context, r *http.Request) (bool, error) {
	cookie, err := r.Cookie(pendingCookieName)
	if err != nil || len(cookie.Value) == 0 || len(cookie.Value) > 192 {
		return false, nil
	}
	if p == nil {
		return false, ErrUnavailable
	}
	pending, _, _, err := p.pending(ctx, r)
	if errors.Is(err, ErrInvalidConsent) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return pending != nil, nil
}
func (p *Provider) csrfToken(signature, browserSignature string, userID uuid.UUID) string {
	mac := hmac.New(sha256.New, p.browserKey)
	mac.Write([]byte("consent\x00" + signature + "\x00" + browserSignature + "\x00" + userID.String()))
	return hex.EncodeToString(mac.Sum(nil))
}
func (p *Provider) Consent(ctx context.Context, r *http.Request, userID uuid.UUID) (*api.OAuthConsent, error) {
	if userID == uuid.Nil() {
		return nil, ErrInvalidConsent
	}
	pending, signature, browser, err := p.pending(ctx, r)
	if err != nil {
		return nil, err
	}
	redirect, err := url.Parse(pending.RedirectURI)
	if err != nil {
		return nil, ErrInvalidConsent
	}
	return &api.OAuthConsent{RegistrationTeamID: pending.Client.RegistrationTeamID, Client: api.OAuthClientSummary{ID: pending.Client.ID, Name: pending.Client.Name, CallbackOrigin: redirect.Scheme + "://" + redirect.Host}, Resource: pending.Resource, Scopes: slices.Clone(pending.Scopes), ExpiresAt: pending.ExpiresAt, CSRFToken: p.csrfToken(signature, browser, userID), Teams: []api.OAuthTeam{}}, nil
}
func (p *Provider) CompleteConsent(w http.ResponseWriter, r *http.Request, userID, teamID uuid.UUID, sites []uuid.UUID, decision, csrf string, authorize func(context.Context, *auth.OAuthGrant, uuid.UUID) error) error {
	if !p.Ready() {
		return ErrUnavailable
	}
	if userID == uuid.Nil() || len(sites) > 512 {
		return ErrInvalidConsent
	}
	pending, signature, browser, err := p.pending(r.Context(), r)
	if err != nil {
		return err
	}
	expected := p.csrfToken(signature, browser, userID)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(csrf)) != 1 {
		return ErrInvalidConsent
	}
	if decision != "approve" && decision != "deny" {
		return ErrInvalidConsent
	}
	if decision == "approve" && pending.Client.RegistrationTeamID != uuid.Nil() && pending.Client.RegistrationTeamID != teamID {
		return ErrConsentForbidden
	}
	if decision == "approve" && (teamID == uuid.Nil() || len(sites) == 0 || authorize == nil) {
		return ErrInvalidConsent
	}
	grant := database.OAuthGrant{ID: uuid.NewV7(), UserID: userID, TeamID: teamID, SiteIDs: slices.Clone(sites), ClientRevision: pending.Client.RegistrationVersion, ClientID: pending.Client.ID, ClientName: pending.Client.Name, ClientURI: pending.Client.URI, Audience: pending.Resource, Scopes: slices.Clone(pending.Scopes), CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(grantLifetime)}
	principal := &auth.OAuthGrant{ID: grant.ID, UserID: userID, TeamID: teamID, SiteIDs: slices.Clone(sites), Scopes: slices.Clone(pending.Scopes), Audience: pending.Resource}
	p.mu.Lock()
	defer p.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), metadataTimeout)
	defer cancel()
	tx, err := p.store.BeginOAuth(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.ConsumePending(ctx, signature, browser); err != nil {
		return ErrInvalidConsent
	}
	if decision == "deny" {
		if err := tx.AppendAudit(ctx, userID, uuid.Nil(), uuid.Nil(), "oauth.consent_denied"); err != nil {
			return err
		}
		if !p.Ready() {
			return ErrUnavailable
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		p.setBrowserCookie(w, pendingCookieName, "", -1)
		p.callback(w, r, pending.RedirectURI, pending.State, "", "access_denied")
		return nil
	}
	seen := make(map[uuid.UUID]bool, len(sites))
	for _, siteID := range sites {
		if siteID == uuid.Nil() || seen[siteID] {
			return ErrInvalidConsent
		}
		seen[siteID] = true
		if err := authorize(ctx, principal, siteID); err != nil {
			return err
		}
	}
	if err := tx.CheckOAuthClient(ctx, pending.Client.ID, pending.Client.RegistrationVersion); err != nil {
		return ErrInvalidConsent
	}
	if err := tx.CreateGrant(ctx, grant); err != nil {
		return err
	}
	op := &providerOperation{tx: tx, client: pending.Client, resource: pending.Resource}
	ctx = context.WithValue(ctx, operationKey{}, op)
	form := url.Values{"response_type": {"code"}, "client_id": {pending.Client.ID}, "redirect_uri": {pending.RedirectURI}, "scope": {strings.Join(pending.Scopes, " ")}, "code_challenge": {pending.Challenge}, "code_challenge_method": {"S256"}, "audience": {pending.Resource}}
	if pending.State != "" {
		form.Set("state", pending.State)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.urls.Issuer+"/oauth/authorize?"+form.Encode(), nil)
	if err != nil {
		return err
	}
	request.Form = form
	ar, err := p.newAuthorizeRequest(ctx, request)
	if err != nil {
		return err
	}
	ar.SetID(grant.ID.String())
	for _, scope := range pending.Scopes {
		ar.GrantScope(scope)
	}
	ar.GrantAudience(pending.Resource)
	session := &oauthSession{Subject: userID.String(), GrantID: grant.ID, GrantExpiry: grant.ExpiresAt}
	response, err := p.core.NewAuthorizeResponse(ctx, ar, session)
	if err != nil {
		return err
	}
	if !p.Ready() || op.rollback {
		return ErrUnavailable
	}
	if err := tx.AppendAudit(ctx, userID, teamID, grant.ID, "oauth.consent_approved"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	p.setBrowserCookie(w, pendingCookieName, "", -1)
	p.callback(w, r, pending.RedirectURI, pending.State, response.GetCode(), "")
	return nil
}
func (p *Provider) newAuthorizeRequest(ctx context.Context, r *http.Request) (fosite.AuthorizeRequester, error) {
	form := r.Form
	if (form.Has("state") && form.Get("state") == "") || len(form.Get("state")) > 1024 || form.Get("code_challenge_method") != "S256" || form.Get("code_challenge") == "" {
		return nil, fosite.ErrInvalidRequest
	}
	request, err := p.core.NewAuthorizeRequest(ctx, r)
	// Fosite v0.49 checks client-state entropy last, after protocol validation.
	if request != nil && errors.Is(err, fosite.ErrInvalidState) {
		return request, nil
	}
	return request, err
}

func (p *Provider) callback(w http.ResponseWriter, r *http.Request, redirect, state, code, errorCode string) {
	target, err := url.Parse(redirect)
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	target = target.Clone()
	query := target.Query()
	query.Set("iss", p.urls.Issuer)
	if state != "" {
		query.Set("state", state)
	} else {
		query.Del("state")
	}
	if errorCode != "" {
		query.Set("error", errorCode)
		query.Del("code")
	} else {
		query.Set("code", code)
		query.Del("error")
	}
	target.RawQuery = query.Encode()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, target.String(), http.StatusSeeOther) // #nosec G710 -- Only exact client redirects validated at authorization or pinned pending-consent validation reach this sink.
}
func (p *Provider) RedirectConsentError(w http.ResponseWriter, r *http.Request, err error) {
	code := "failed"
	if errors.Is(err, ErrUnavailable) || errors.Is(err, database.ErrOAuthConflict) {
		code = "unavailable"
	} else if errors.Is(err, ErrInvalidConsent) || errors.Is(err, database.ErrOAuthNotFound) {
		code = "expired"
	} else if errors.Is(err, database.ErrOAuthInactive) || errors.Is(err, ErrConsentForbidden) {
		code = "forbidden"
	}
	http.Redirect(w, r, p.urls.Issuer+"/oauth/consent?error="+code, http.StatusSeeOther)
}
