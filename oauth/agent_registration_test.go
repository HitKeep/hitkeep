package oauth

import (
	"context"
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
	"uuid"

	"hitkeep/api"
	"hitkeep/auth"
)

type agentRegistrationReply struct {
	ID          string `json:"registration_id"`
	ClaimToken  string `json:"claim_token"`
	Assertion   string `json:"identity_assertion"`
	AccessToken string `json:"access_token"`
	Claim       struct {
		Code     string `json:"user_code"`
		URI      string `json:"verification_uri"`
		Interval int    `json:"interval"`
	} `json:"claim"`
}

func newAgentFixture(t *testing.T) *providerFixture {
	t.Helper()
	f := newProviderFixture(t)
	f.options.AgentActionsEnabled = func() bool { return true }
	var err error
	f.p, err = New(f.store, f.options)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func registerAgent(t *testing.T, f *providerFixture, resource, scope string) agentRegistrationReply {
	t.Helper()
	body, err := json.Marshal(map[string]string{"type": "service_auth", "login_hint": "oauth@example.test", "resource": resource, "scope": scope})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", f.p.Issuer()+"/agent/identity", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.p.AgentIdentity(w, r)
	var reply agentRegistrationReply
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || w.Code != 201 {
		t.Fatalf("register: %d %s %v", w.Code, w.Body.String(), err)
	}
	if reply.AccessToken != "" || reply.Assertion != "" || len(reply.Claim.Code) != 6 || reply.Claim.Interval != agentPollInterval {
		t.Fatalf("preclaim credential or invalid ceremony: %s", w.Body.String())
	}
	return reply
}

func agentBrowser(t *testing.T, f *providerFixture, reply agentRegistrationReply) (*http.Request, *api.OAuthConsent) {
	t.Helper()
	w := httptest.NewRecorder()
	f.p.StartAgentClaim(w, httptest.NewRequest("GET", reply.Claim.URI, nil))
	if w.Code != 303 || w.Header().Get("Location") != f.p.Issuer()+"/oauth/consent" {
		t.Fatalf("claim bootstrap: %d %s", w.Code, w.Body.String())
	}
	r := httptest.NewRequest("POST", f.p.Issuer()+"/api/oauth/consent", nil)
	r.PostForm = url.Values{"claim_code": {reply.Claim.Code}}
	for _, cookie := range w.Result().Cookies() {
		r.AddCookie(cookie)
	}
	info, err := f.p.Consent(t.Context(), r, f.userID)
	if err != nil || !info.AgentClaim {
		t.Fatalf("claim consent: %v %+v", err, info)
	}
	return r, info
}

func agentAuthority(f *providerFixture) func(context.Context, *auth.OAuthGrant, uuid.UUID) error {
	return func(_ context.Context, grant *auth.OAuthGrant, siteID uuid.UUID) error {
		if grant.UserID != f.userID || grant.TeamID != f.teamID || (siteID != f.siteID && !(siteID == uuid.Nil() && len(grant.Scopes) == 1 && grant.Scopes[0] == ScopeBillingWrite)) {
			return ErrConsentForbidden
		}
		return nil
	}
}

func agentTokenRequest(t *testing.T, f *providerFixture, form url.Values, authorize func(context.Context, *auth.OAuthGrant, uuid.UUID) error) (int, map[string]any) {
	t.Helper()
	r := httptest.NewRequest("POST", f.p.Issuer()+"/oauth/token", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	f.p.TokenWithAuthority(w, r, authorize)
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("token JSON: %d %s", w.Code, w.Body.String())
	}
	return w.Code, response
}

func editAgentPending(t *testing.T, f *providerFixture, reply agentRegistrationReply, edit func(*agentRegistration)) {
	t.Helper()
	tx, err := f.store.BeginOAuth(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	state, pending, err := f.p.loadAgentPending(t.Context(), tx, tokenSignature(reply.ClaimToken), "")
	if err != nil {
		t.Fatal(err)
	}
	edit(state)
	pending.Payload, err = json.Marshal(state)
	if err == nil {
		err = tx.UpdateAgentPending(t.Context(), *pending)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		t.Fatal(err)
	}
}

func approveAgent(t *testing.T, f *providerFixture, reply agentRegistrationReply) {
	t.Helper()
	r, info := agentBrowser(t, f, reply)
	w := httptest.NewRecorder()
	if err := f.p.CompleteConsent(w, r, f.userID, f.teamID, []uuid.UUID{f.siteID}, "approve", info.CSRFToken, agentAuthority(f)); err != nil {
		t.Fatal(err)
	}
	if w.Code != 303 || !strings.Contains(w.Header().Get("Location"), "claim=complete") {
		t.Fatalf("claim completion: %d %s", w.Code, w.Header().Get("Location"))
	}
	if err := f.p.CompleteConsent(httptest.NewRecorder(), r, f.userID, f.teamID, []uuid.UUID{f.siteID}, "approve", info.CSRFToken, agentAuthority(f)); err == nil {
		t.Fatal("claim replay created another grant")
	}
}

func TestAgentRegistrationWithholdsCredentialsAndPersistsOnlyHashedHandles(t *testing.T) {
	f := newAgentFixture(t)
	reply := registerAgent(t, f, "", "")
	grants, err := f.store.ListOAuthGrants(t.Context(), f.userID)
	if err != nil || len(grants) != 0 {
		t.Fatalf("preclaim grants: %v %+v", err, grants)
	}
	var count int
	if err := f.store.DB().QueryRowContext(t.Context(), "SELECT count(*) FROM oauth_credentials").Scan(&count); err != nil || count != 0 {
		t.Fatalf("preclaim credentials=%d: %v", count, err)
	}
	var signature, attempt, payload string
	if err := f.store.DB().QueryRowContext(t.Context(), "SELECT signature,browser_signature,payload FROM oauth_pending").Scan(&signature, &attempt, &payload); err != nil {
		t.Fatal(err)
	}
	verification, err := url.Parse(reply.Claim.URI)
	if err != nil {
		t.Fatal(err)
	}
	if signature != tokenSignature(reply.ClaimToken) || attempt != tokenSignature(verification.Query().Get("claim_attempt_token")) || strings.Contains(payload, reply.ClaimToken) || strings.Contains(payload, verification.Query().Get("claim_attempt_token")) {
		t.Fatal("raw claim handle persisted")
	}
	var state agentRegistration
	if err := json.Unmarshal([]byte(payload), &state); err != nil || state.CodeSignature != tokenSignature(reply.ID+"\x00"+reply.Claim.Code) {
		t.Fatalf("code not hashed: %v", err)
	}
	form := url.Values{"grant_type": {agentClaimGrant}, "claim_token": {reply.ClaimToken}}
	status, response := agentTokenRequest(t, f, form, agentAuthority(f))
	if status != 400 || response["error"] != "authorization_pending" || response["access_token"] != nil || response["identity_assertion"] != nil {
		t.Fatalf("preclaim token: %d %+v", status, response)
	}
	status, response = agentTokenRequest(t, f, form, agentAuthority(f))
	if status != 400 || response["error"] != "slow_down" {
		t.Fatalf("poll throttle: %d %+v", status, response)
	}
	p2, err := New(f.store, f.options)
	if err != nil {
		t.Fatal(err)
	}
	f.p = p2
	_, response = agentTokenRequest(t, f, form, agentAuthority(f))
	if response["error"] != "slow_down" {
		t.Fatal("poll throttling did not survive provider restart")
	}
}

func TestAgentClaimRefreshAndBothRevocationLayers(t *testing.T) {
	f := newAgentFixture(t)
	reply := registerAgent(t, f, f.p.Resource("api"), "sites:read analytics:read")
	approveAgent(t, f, reply)
	form := url.Values{"grant_type": {agentClaimGrant}, "claim_token": {reply.ClaimToken}}
	status, tokens := agentTokenRequest(t, f, form, agentAuthority(f))
	if status != 200 || tokens["refresh_token"] != nil || tokens["access_token"] == nil || tokens["identity_assertion"] == nil {
		t.Fatalf("claim token: %d %+v", status, tokens)
	}
	access, assertion := tokens["access_token"].(string), tokens["identity_assertion"].(string)
	grant, err := f.p.Authenticate(t.Context(), access, f.p.Resource("api"))
	if err != nil || grant.UserID != f.userID || grant.TeamID != f.teamID || len(grant.SiteIDs) != 1 || grant.SiteIDs[0] != f.siteID {
		t.Fatalf("wrong agent principal: %+v %v", grant, err)
	}
	if _, err := f.p.Authenticate(t.Context(), access, f.p.Resource("mcp")); err == nil {
		t.Fatal("API credential accepted on MCP")
	}
	if _, replay := agentTokenRequest(t, f, form, agentAuthority(f)); replay["access_token"] != nil {
		t.Fatal("claim poll replay returned credentials")
	}
	r := httptest.NewRequest("POST", f.p.Issuer()+"/oauth/revoke", strings.NewReader(url.Values{"token": {access}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	f.p.Revoke(w, r)
	if w.Code != 200 {
		t.Fatalf("agent token-only revoke: %d %s", w.Code, w.Body.String())
	}
	if _, err := f.p.Authenticate(t.Context(), access, f.p.Resource("api")); err == nil {
		t.Fatal("revoked access token active")
	}
	refresh := url.Values{"grant_type": {agentJWTGrant}, "assertion": {assertion}, "resource": {f.p.Resource("api")}}
	status, renewed := agentTokenRequest(t, f, refresh, agentAuthority(f))
	if status != 200 || renewed["access_token"] == nil || renewed["refresh_token"] != nil {
		t.Fatalf("own assertion did not refresh after access revocation: %d %+v", status, renewed)
	}
	if err := f.store.RevokeOAuthGrant(t.Context(), f.userID, grant.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.p.Authenticate(t.Context(), renewed["access_token"].(string), f.p.Resource("api")); err == nil {
		t.Fatal("grant revocation did not kill access")
	}
	if status, response := agentTokenRequest(t, f, refresh, agentAuthority(f)); status == 200 || response["access_token"] != nil {
		t.Fatal("revoked registration assertion refreshed")
	}
}

func TestAgentConsentRejectsWrongHumanCodeCSRFAndAuthority(t *testing.T) {
	f := newAgentFixture(t)
	reply := registerAgent(t, f, "", "")
	r, info := agentBrowser(t, f, reply)
	wrongUser := uuid.NewV7()
	_, signature, browser, err := f.p.pending(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.p.CompleteConsent(httptest.NewRecorder(), r, wrongUser, f.teamID, []uuid.UUID{f.siteID}, "approve", f.p.csrfToken(signature, browser, wrongUser), agentAuthority(f)); !errors.Is(err, ErrConsentForbidden) {
		t.Fatalf("email mismatch not rejected: %v", err)
	}
	if err := f.p.CompleteConsent(httptest.NewRecorder(), r, f.userID, f.teamID, []uuid.UUID{f.siteID}, "approve", "wrong", agentAuthority(f)); err == nil {
		t.Fatal("wrong CSRF accepted")
	}
	if err := f.p.CompleteConsent(httptest.NewRecorder(), r, f.userID, uuid.NewV7(), []uuid.UUID{f.siteID}, "approve", info.CSRFToken, agentAuthority(f)); err == nil {
		t.Fatal("wrong team accepted")
	}
	if err := f.p.CompleteConsent(httptest.NewRecorder(), r, f.userID, f.teamID, []uuid.UUID{uuid.NewV7()}, "approve", info.CSRFToken, agentAuthority(f)); err == nil {
		t.Fatal("wrong site accepted")
	}
	bad := "000000"
	if reply.Claim.Code == bad {
		bad = "111111"
	}
	r.PostForm.Set("claim_code", bad)
	for range agentCodeAttempts {
		if err := f.p.CompleteConsent(httptest.NewRecorder(), r, f.userID, f.teamID, []uuid.UUID{f.siteID}, "approve", info.CSRFToken, agentAuthority(f)); !errors.Is(err, ErrAgentClaimCode) {
			t.Fatalf("wrong code: %v", err)
		}
	}
	r.PostForm.Set("claim_code", reply.Claim.Code)
	if err := f.p.CompleteConsent(httptest.NewRecorder(), r, f.userID, f.teamID, []uuid.UUID{f.siteID}, "approve", info.CSRFToken, agentAuthority(f)); err == nil {
		t.Fatal("code lockout bypassed")
	}
	grants, err := f.store.ListOAuthGrants(t.Context(), f.userID)
	if err != nil || len(grants) != 0 {
		t.Fatal("failed consent created a grant")
	}
}

func TestAgentClaimDenialExpiryRotationAndResourceIsolation(t *testing.T) {
	f := newAgentFixture(t)
	reply := registerAgent(t, f, "", "")
	r, info := agentBrowser(t, f, reply)
	if err := f.p.CompleteConsent(httptest.NewRecorder(), r, f.userID, uuid.Nil(), nil, "deny", info.CSRFToken, nil); err != nil {
		t.Fatal(err)
	}
	_, denied := agentTokenRequest(t, f, url.Values{"grant_type": {agentClaimGrant}, "claim_token": {reply.ClaimToken}}, agentAuthority(f))
	if denied["error"] != "access_denied" || denied["access_token"] != nil {
		t.Fatalf("deny: %+v", denied)
	}
	expired := registerAgent(t, f, "", "")
	editAgentPending(t, f, expired, func(state *agentRegistration) { state.CodeExpiresAt = time.Now().Add(-time.Minute) })
	_, response := agentTokenRequest(t, f, url.Values{"grant_type": {agentClaimGrant}, "claim_token": {expired.ClaimToken}}, agentAuthority(f))
	if response["error"] != "expired_token" {
		t.Fatalf("code expiry: %+v", response)
	}
	request := httptest.NewRequest("POST", f.p.Issuer()+"/agent/identity/claim", strings.NewReader(`{"claim_token":"`+expired.ClaimToken+`","email":"oauth@example.test"}`))
	request.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.p.AgentClaim(w, request)
	if w.Code != 200 {
		t.Fatalf("restart claim: %d %s", w.Code, w.Body.String())
	}
	old := httptest.NewRecorder()
	f.p.StartAgentClaim(old, httptest.NewRequest("GET", expired.Claim.URI, nil))
	if old.Code == 303 {
		t.Fatal("rotated verification URL still active")
	}
	approved := registerAgent(t, f, f.p.Resource("mcp"), "sites:read analytics:read")
	approveAgent(t, f, approved)
	form := url.Values{"grant_type": {agentClaimGrant}, "claim_token": {approved.ClaimToken}, "resource": {f.p.Resource("api")}}
	if status, response := agentTokenRequest(t, f, form, agentAuthority(f)); status == 200 || response["access_token"] != nil {
		t.Fatal("resource substitution accepted")
	}
	form.Set("resource", f.p.Resource("mcp"))
	form.Set("scope", "sites:read analytics:read sites:write")
	if status, _ := agentTokenRequest(t, f, form, agentAuthority(f)); status == 200 {
		t.Fatal("scope escalation accepted")
	}
}

func TestAgentRefreshRechecksAuthorityAndAIFlag(t *testing.T) {
	f := newAgentFixture(t)
	reply := registerAgent(t, f, "", "")
	approveAgent(t, f, reply)
	_, tokens := agentTokenRequest(t, f, url.Values{"grant_type": {agentClaimGrant}, "claim_token": {reply.ClaimToken}}, agentAuthority(f))
	assertion, access := tokens["identity_assertion"].(string), tokens["access_token"].(string)
	form := url.Values{"grant_type": {agentJWTGrant}, "assertion": {assertion}}
	if status, response := agentTokenRequest(t, f, form, func(context.Context, *auth.OAuthGrant, uuid.UUID) error { return ErrConsentForbidden }); status == 200 || response["access_token"] != nil {
		t.Fatal("lost current authority refreshed")
	}
	f.p.agentActionsEnabled = func() bool { return false }
	if _, err := f.p.Authenticate(t.Context(), access, f.p.Resource("api")); err == nil {
		t.Fatal("AI-off agent access active")
	}
	if status, response := agentTokenRequest(t, f, form, agentAuthority(f)); status == 200 || response["access_token"] != nil {
		t.Fatal("AI-off assertion refreshed")
	}
	w := httptest.NewRecorder()
	f.p.Metadata(w, httptest.NewRequest("GET", "/", nil))
	if strings.Contains(w.Body.String(), "agent_auth") {
		t.Fatal("AI-off registration metadata advertised")
	}
}

func TestAgentRegistrationRejectsUnsupportedMethodsAndMalformedInputs(t *testing.T) {
	f := newAgentFixture(t)
	for _, body := range []string{
		`{"type":"anonymous"}`,
		`{"type":"identity_assertion","login_hint":"oauth@example.test"}`,
		`{"type":"service_auth","login_hint":"Name <oauth@example.test>"}`,
		`{"type":"service_auth","login_hint":"oauth@example.test","resource":"https://other.example/api"}`,
		`{"type":"service_auth","login_hint":"oauth@example.test","scope":"sites:read sites:read"}`,
		`{"type":"service_auth","login_hint":"oauth@example.test","unexpected":true}`,
		`{"type":"anonymous","type":"service_auth","login_hint":"oauth@example.test"}`,
		`{"type":"service_auth","login_hint":"oauth@example.test"} {}`,
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", f.p.Issuer()+"/agent/identity", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		f.p.AgentIdentity(w, r)
		if w.Code != 400 || strings.Contains(w.Body.String(), "claim_token") || strings.Contains(w.Body.String(), "identity_assertion\":") {
			t.Fatalf("invalid registration accepted: %d %s", w.Code, w.Body.String())
		}
	}
	f.p.agentActionsEnabled = func() bool { return false }
	for _, run := range []func(http.ResponseWriter, *http.Request){f.p.AgentIdentity, f.p.AgentClaim, f.p.StartAgentClaim} {
		w := httptest.NewRecorder()
		run(w, httptest.NewRequest("POST", "/", nil))
		if w.Code != 404 {
			t.Fatalf("AI-off agent endpoint=%d", w.Code)
		}
	}
	var count int
	if err := f.store.DB().QueryRowContext(t.Context(), "SELECT count(*) FROM oauth_pending").Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid registration created state=%d: %v", count, err)
	}
}

func TestLegacyVerifiedEmailShapeUsesTheSameNativeClaimBoundary(t *testing.T) {
	f := newAgentFixture(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", f.p.Issuer()+"/agent/identity", strings.NewReader(`{"type":"identity_assertion","assertion_type":"verified_email","assertion":"oauth@example.test"}`))
	r.Header.Set("Content-Type", "application/json")
	f.p.AgentIdentity(w, r)
	var reply agentRegistrationReply
	var kind struct {
		Type string `json:"registration_type"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &kind); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || kind.Type != "email-verification" || reply.AccessToken != "" || reply.Assertion != "" {
		t.Fatalf("legacy ceremony: %d %s", w.Code, w.Body.String())
	}
	form := url.Values{"grant_type": {agentClaimGrant}, "claim_token": {reply.ClaimToken}}
	if _, pending := agentTokenRequest(t, f, form, agentAuthority(f)); pending["error"] != "authorization_pending" || pending["access_token"] != nil {
		t.Fatalf("legacy preclaim credentials: %+v", pending)
	}
	browser, info := agentBrowser(t, f, reply)
	browser.PostForm.Set("claim_code", "bad-code")
	if err := f.p.CompleteConsent(httptest.NewRecorder(), browser, f.userID, f.teamID, []uuid.UUID{f.siteID}, "approve", info.CSRFToken, agentAuthority(f)); !errors.Is(err, ErrAgentClaimCode) {
		t.Fatalf("legacy bypassed human code: %v", err)
	}
	browser.PostForm.Set("claim_code", reply.Claim.Code)
	if err := f.p.CompleteConsent(httptest.NewRecorder(), browser, f.userID, f.teamID, []uuid.UUID{f.siteID}, "approve", info.CSRFToken, agentAuthority(f)); err != nil {
		t.Fatal(err)
	}
	editAgentPending(t, f, reply, func(state *agentRegistration) { state.NextPoll = time.Time{} })
	status, tokens := agentTokenRequest(t, f, form, agentAuthority(f))
	if status != 200 || tokens["access_token"] == nil || tokens["identity_assertion"] == nil || tokens["refresh_token"] != nil {
		t.Fatalf("legacy postclaim credentials: %d %+v", status, tokens)
	}
	if _, replay := agentTokenRequest(t, f, form, agentAuthority(f)); replay["access_token"] != nil {
		t.Fatal("legacy claim replay")
	}
	grant, err := f.p.Authenticate(t.Context(), tokens["access_token"].(string), f.p.Resource("api"))
	if err != nil {
		t.Fatal(err)
	}
	refresh := url.Values{"grant_type": {agentJWTGrant}, "assertion": {tokens["identity_assertion"].(string)}}
	if status, _ := agentTokenRequest(t, f, refresh, agentAuthority(f)); status != 200 {
		t.Fatal("legacy assertion cannot refresh")
	}
	if err := f.store.RevokeOAuthGrant(t.Context(), f.userID, grant.ID); err != nil {
		t.Fatal(err)
	}
	if status, _ := agentTokenRequest(t, f, refresh, agentAuthority(f)); status == 200 {
		t.Fatal("legacy grant revocation bypassed")
	}
	f.p.agentActionsEnabled = func() bool { return false }
	disabled := httptest.NewRecorder()
	f.p.AgentIdentity(disabled, r)
	if disabled.Code != 404 {
		t.Fatal("AI-off legacy registration active")
	}
}

func TestAgentMetadataAdvertisesOnlyImplementedEmailMethods(t *testing.T) {
	f := newAgentFixture(t)
	w := httptest.NewRecorder()
	f.p.Metadata(w, httptest.NewRequest("GET", "/", nil))
	var metadata struct {
		Agent struct {
			Types            []string            `json:"identity_types_supported"`
			Identity         map[string][]string `json:"identity_assertion"`
			IdentityEndpoint string              `json:"identity_endpoint"`
			RegisterURI      string              `json:"register_uri"`
			ClaimEndpoint    string              `json:"claim_endpoint"`
			ClaimURI         string              `json:"claim_uri"`
		} `json:"agent_auth"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	agent := metadata.Agent
	if strings.Join(agent.Types, " ") != "service_auth identity_assertion" || strings.Join(agent.Identity["assertion_types_supported"], " ") != "verified_email" || strings.Join(agent.Identity["credential_types_supported"], " ") != "access_token" || agent.IdentityEndpoint != agent.RegisterURI || agent.ClaimEndpoint != agent.ClaimURI {
		t.Fatalf("inaccurate methods or aliases: %+v", agent)
	}
	for _, assertionType := range []string{"urn:ietf:params:oauth:token-type:id-jag", "jwt", "other"} {
		body, _ := json.Marshal(map[string]string{"type": "identity_assertion", "assertion_type": assertionType, "assertion": "oauth@example.test"})
		r := httptest.NewRequest("POST", "/agent/identity", strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		denied := httptest.NewRecorder()
		f.p.AgentIdentity(denied, r)
		if denied.Code != 400 {
			t.Fatalf("unsupported assertion accepted: %s", assertionType)
		}
	}
}
