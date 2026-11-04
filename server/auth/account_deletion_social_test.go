package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"github.com/go-jose/go-jose/v4"
	"golang.org/x/oauth2"

	"hitkeep/database"
	json "hitkeep/jsonapi"
	"hitkeep/security"
	"hitkeep/server/shared"
	"hitkeep/socialauth"
	"hitkeep/sso"
)

// The transport supplies signed provider responses; production verification,
// browser state, subject lookup, challenge ownership and factors all run normally.
type deletionSocialProvider struct {
	name, nonce, challenge, subject, email string
	key                                    *rsa.PrivateKey
}

const deletionMicrosoftTenant = "11111111-2222-3333-4444-555555555555"
const deletionMicrosoftObject = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

func (f *deletionSocialProvider) RoundTrip(r *http.Request) (*http.Response, error) {
	out := httptest.NewRecorder()
	out.Header().Set("Content-Type", "application/json")
	issuer := "https://accounts.google.com"
	if f.name == "microsoft" {
		issuer = "https://login.microsoftonline.com/" + deletionMicrosoftTenant + "/v2.0"
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/.well-known/openid-configuration"):
		metadataIssuer := issuer
		if f.name == "microsoft" {
			metadataIssuer = "https://login.microsoftonline.com/{tenantid}/v2.0"
		}
		_ = json.MarshalWrite(out, map[string]any{"issuer": metadataIssuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{oidc.RS256}})
	case strings.HasSuffix(r.URL.Path, "/keys"):
		encoded, _ := json.Marshal(jose.JSONWebKey{Key: &f.key.PublicKey, KeyID: "deletion-provider", Algorithm: oidc.RS256, Use: "sig"})
		var key map[string]any
		_ = json.Unmarshal(encoded, &key)
		key["issuer"] = "https://login.microsoftonline.com/{tenantid}/v2.0"
		_ = json.MarshalWrite(out, map[string]any{"keys": []any{key}})
	case strings.HasSuffix(r.URL.Path, "/token") || r.URL.Path == "/login/oauth/access_token":
		if err := r.ParseForm(); err != nil {
			return nil, err
		}
		if r.Form.Get("code") != "fixture-code" || oauth2.S256ChallengeFromVerifier(r.Form.Get("code_verifier")) != f.challenge {
			return nil, fmt.Errorf("provider code or PKCE mismatch")
		}
		payload := map[string]any{"access_token": "fixture-provider-token", "token_type": "Bearer"}
		if f.name != "github" {
			claims := map[string]any{"iss": issuer, "sub": f.subject, "aud": "deletion-client", "exp": time.Now().Add(time.Minute).Unix(), "iat": time.Now().Unix(), "nonce": f.nonce, "email": f.email, "email_verified": true, "tid": deletionMicrosoftTenant, "oid": deletionMicrosoftObject}
			encoded, _ := json.Marshal(claims)
			payload["id_token"] = oidctest.SignIDToken(f.key, "deletion-provider", oidc.RS256, string(encoded))
		}
		_ = json.MarshalWrite(out, payload)
	case r.URL.Path == "/user":
		_ = json.MarshalWrite(out, map[string]any{"id": 42, "login": "fixture-user"})
	case r.URL.Path == "/user/emails":
		_ = json.MarshalWrite(out, []any{map[string]any{"email": f.email, "primary": true, "verified": true}})
	default:
		return nil, fmt.Errorf("unexpected provider fixture route")
	}
	return out.Result(), nil
}

func TestAccountDeletionSocialVerifiesLinkedProviderAndLocalMFA(t *testing.T) {
	for _, provider := range []string{"google", "github", "microsoft"} {
		t.Run(provider, func(t *testing.T) {
			h, store := setupAuthTestEnv(t)
			defer store.Close()
			id, err := store.CreateUser(t.Context(), "delete-social@example.test", "unused")
			if err != nil {
				t.Fatal(err)
			}
			const secret = "JBSWY3DPEHPK3PXP"
			if err := store.EnableUserTOTP(t.Context(), id, secret); err != nil {
				t.Fatal(err)
			}
			key, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				t.Fatal(err)
			}
			fixture := &deletionSocialProvider{name: provider, subject: "deletion-subject", email: "delete-social@example.test", key: key}
			subject := fixture.subject
			if provider == "github" {
				subject = "42"
			}
			if provider == "microsoft" {
				subject = deletionMicrosoftTenant + ":" + deletionMicrosoftObject
			}

			h.ctx.SocialAuth = socialauth.NewClient(sso.NewClient(&http.Client{Transport: fixture}))
			h.ctx.Config.SocialGoogleClientID = "deletion-client"
			h.ctx.Config.SocialGoogleClientSecret = "fixture-secret"
			h.ctx.Config.SocialGitHubClientID = "deletion-client"
			h.ctx.Config.SocialGitHubClientSecret = "fixture-secret"
			h.ctx.Config.SocialMicrosoftClientID = "deletion-client"
			h.ctx.Config.SocialMicrosoftClientSecret = "fixture-secret"
			attempt, hash := deletionAttemptFixture(t, h, id)
			target := shared.AccountDeletionReturnPath(attempt)
			unlinked := completeDeletionSocialProvider(t, h, fixture, target)
			if unlinked.Code != 403 || len(unlinked.Result().Cookies()) != 0 {
				t.Fatal("deletion accepted email-only provider linking")
			}
			if linked, err := store.GetSocialIdentity(t.Context(), provider, subject); err != nil || linked != nil {
				t.Fatal("deletion reauthentication linked an identity")
			}
			if _, err := store.LinkSocialIdentity(t.Context(), database.LinkSocialIdentityInput{UserID: id, Provider: provider, Subject: subject, ObservedEmail: fixture.email}); err != nil {
				t.Fatal(err)
			}
			out := completeDeletionSocialProvider(t, h, fixture, target)
			if out.Code != 200 || len(out.Result().Cookies()) != 0 {
				t.Fatalf("complete %d %s", out.Code, out.Body.String())
			}
			var response loginResponse
			if err := json.Unmarshal(out.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Status != "mfa_required" {
				t.Fatal("social skipped local MFA")
			}
			code, err := security.GenerateCurrentTOTPCode(secret, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(mfaTotpVerifyRequest{ChallengeToken: response.ChallengeToken, Code: code})
			completed := httptest.NewRecorder()
			h.handleMFATOTPVerify()(completed, deletionJSONRequest("/api/auth/mfa/totp/verify", body))
			if completed.Code != 200 || !h.ctx.AuthState.AccountDeletionProofVerified(attempt, id, responseSessionID(t, h, completed), hash) {
				t.Fatalf("factor %d %s", completed.Code, completed.Body.String())
			}
		})
	}
}

func completeDeletionSocialProvider(t *testing.T, h *handler, fixture *deletionSocialProvider, target string) *httptest.ResponseRecorder {
	t.Helper()
	provider := fixture.name
	body, _ := json.Marshal(map[string]string{"flow": "login", "return_url": target})
	req := deletionJSONRequest("/api/auth/social/"+provider+"/start", body)
	req.SetPathValue("provider", provider)
	started := httptest.NewRecorder()
	h.handleSocialStart(false)(started, req)
	if started.Code != 200 {
		t.Fatalf("start %d %s", started.Code, started.Body.String())
	}
	var start struct {
		AuthURL string `json:"auth_url"`
	}
	if err := json.Unmarshal(started.Body.Bytes(), &start); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(start.AuthURL)
	fixture.nonce = u.Query().Get("nonce")
	fixture.challenge = u.Query().Get("code_challenge")
	req = httptest.NewRequest("GET", "/api/auth/social/"+provider+"/callback?state="+u.Query().Get("state")+"&code=fixture-code", nil)
	req.SetPathValue("provider", provider)
	for _, cookie := range started.Result().Cookies() {
		req.AddCookie(cookie)
	}
	callback := httptest.NewRecorder()
	h.handleSocialCallback()(callback, req)
	if callback.Code != 303 {
		t.Fatalf("callback %d", callback.Code)
	}
	u, _ = url.Parse(callback.Header().Get("Location"))
	fragment, _ := url.ParseQuery(u.Fragment)
	token := fragment.Get("social_token")
	if token == "" {
		t.Fatalf("provider verification failed: %s", u.Query().Get("error"))
	}
	body, _ = json.Marshal(map[string]string{"completion_token": token})
	out := httptest.NewRecorder()
	h.handleSocialComplete()(out, deletionJSONRequest("/api/auth/social/complete", body))
	return out
}
