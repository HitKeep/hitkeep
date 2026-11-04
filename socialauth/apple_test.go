package socialauth

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"hitkeep/config"
	json "hitkeep/jsonapi"
	"hitkeep/sso"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAppleConfigurationAndClaims(t *testing.T) {
	conf := &config.Config{PublicURL: "https://example.test/prefix", SocialAppleClientID: "service", SocialAppleClientSecret: "signed-secret"}
	if _, err := ConfigForProvider(conf, ProviderApple); !errors.Is(err, ErrNotConfigured) {
		t.Fatal("missing encryption key accepted")
	}
	conf.SocialAppleTokenEncryptionKey = strings.Repeat("k", 32)
	cfg, err := ConfigForProvider(conf, ProviderApple)
	if err != nil || cfg.RedirectURL != "https://example.test/prefix/api/auth/social/apple/callback" {
		t.Fatalf("config: %+v %v", cfg, err)
	}
	for _, value := range []any{true, "true"} {
		identity, err := appleIdentity(&oidc.IDToken{Subject: "immutable"}, map[string]any{"email": "alias@private.icloud.com", "email_verified": value})
		if err != nil || !identity.EmailVerified || identity.Subject != "immutable" {
			t.Fatalf("claims: %+v %v", identity, err)
		}
	}
	for _, value := range []any{false, "false", 1, "TRUE", nil} {
		identity, err := appleIdentity(&oidc.IDToken{Subject: "immutable"}, map[string]any{"email": "alias@privaterelay.appleid.com", "email_verified": value})
		if err != nil || identity.EmailVerified {
			t.Fatalf("unverified claims: %+v %v", identity, err)
		}
	}
}

func TestAppleCredentialEncryptionBindsIdentityAndClient(t *testing.T) {
	key := strings.Repeat("k", 32)
	sealed, err := SealAppleRefreshToken(key, "service", "subject", "private-refresh")
	if err != nil || strings.Contains(sealed, "private-refresh") {
		t.Fatalf("seal failed: %v", err)
	}
	token, err := OpenAppleRefreshToken(key, "service", "subject", sealed)
	if err != nil || token != "private-refresh" {
		t.Fatal("roundtrip failed")
	}
	for _, input := range [][3]string{{key, "wrong", "subject"}, {key, "service", "wrong"}, {strings.Repeat("x", 32), "service", "subject"}} {
		if _, err := OpenAppleRefreshToken(input[0], input[1], input[2], sealed); err == nil {
			t.Fatal("credential binding bypass")
		}
	}
}

func TestAppleOIDCCodeFlowAndRevocation(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &oidctest.Server{PublicKeys: []oidctest.PublicKey{{PublicKey: &key.PublicKey, KeyID: "apple-fixture", Algorithm: oidc.RS256}}}
	var server *httptest.Server
	claims := map[string]any{}
	revokeStatus := http.StatusOK
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if r.ParseForm() != nil || r.PostForm.Get("client_secret") != "signed-secret" || r.PostForm.Get("client_id") != "service" || r.Header.Get("Authorization") != "" || r.PostForm.Get("code_verifier") != "" {
				t.Error("Apple token authentication/PKCE mismatch")
				http.Error(w, "invalid", 400)
				return
			}
			raw, _ := json.Marshal(claims)
			token := oidctest.SignIDToken(key, "apple-fixture", oidc.RS256, string(raw))
			w.Header().Set("Content-Type", "application/json")
			_ = json.MarshalWrite(w, map[string]any{"access_token": "discarded-access", "token_type": "Bearer", "refresh_token": "current-refresh", "id_token": token})
		case "/revoke":
			if r.ParseForm() != nil || r.PostForm.Get("token") != "current-refresh" || r.PostForm.Get("token_type_hint") != "refresh_token" || r.PostForm.Get("client_secret") != "signed-secret" {
				t.Error("revocation binding mismatch")
			}
			w.WriteHeader(revokeStatus)
		default:
			fixture.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(server.Close)
	fixture.SetIssuer(server.URL)
	client := NewClient(sso.NewClient(server.Client()))
	cfg := ProviderConfig{Provider: ProviderApple, ClientID: "service", ClientSecret: "signed-secret", RedirectURL: "https://hitkeep.test/api/auth/social/apple/callback", IssuerURL: server.URL, RevokeURL: server.URL + "/revoke"}
	request, err := client.Begin(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(request.URL("state"))
	q := parsed.Query()
	if q.Get("scope") != "email" || q.Get("response_mode") != "form_post" || q.Get("response_type") != "code" || q.Get("nonce") != request.FlowState.Nonce || q.Has("code_challenge") {
		t.Fatalf("unexpected authorization parameters: %v", q)
	}
	reset := func() {
		claims = map[string]any{"iss": server.URL, "sub": "immutable", "aud": "service", "exp": time.Now().Add(time.Minute).Unix(), "nonce": request.FlowState.Nonce, "email": "alias@private.icloud.com", "email_verified": "true"}
	}
	reset()
	identity, err := client.Complete(t.Context(), cfg, request.FlowState, "code")
	if err != nil || identity.AppleRefreshToken != "current-refresh" || !identity.EmailVerified {
		t.Fatalf("Apple completion failed: %v", err)
	}
	payload, _ := json.Marshal(identity)
	if strings.Contains(string(payload), "current-refresh") || strings.Contains(string(payload), "discarded-access") {
		t.Fatal("credential leaked to JSON")
	}
	for _, invalid := range []struct {
		key   string
		value any
	}{{"aud", "wrong-service"}, {"iss", "https://invalid.test"}, {"nonce", "wrong-nonce"}, {"exp", time.Now().Add(-time.Minute).Unix()}} {
		reset()
		claims[invalid.key] = invalid.value
		if _, err := client.Complete(t.Context(), cfg, request.FlowState, "code"); !errors.Is(err, ErrTokenValidation) {
			t.Fatalf("accepted invalid %s: %v", invalid.key, err)
		}
	}
	if err := client.RevokeApple(t.Context(), cfg, "service", "current-refresh"); err != nil {
		t.Fatal(err)
	}
	revokeStatus = http.StatusServiceUnavailable
	if err := client.RevokeApple(t.Context(), cfg, "service", "current-refresh"); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatal("provider failure not sanitized")
	}
	if err := client.RevokeApple(t.Context(), cfg, "different-service", "current-refresh"); !errors.Is(err, ErrNotConfigured) {
		t.Fatal("wrong stored client accepted")
	}
}
