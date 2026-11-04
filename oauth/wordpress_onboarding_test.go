package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestWordPressSignupKeepsPendingAuthorizationAndDomainHint(t *testing.T) {
	f := newProviderFixture(t)
	verifier := strings.Repeat("v", 64)
	challenge := sha256.Sum256([]byte(verifier))
	query := url.Values{"response_type": {"code"}, "client_id": {"test-client"}, "redirect_uri": {"https://client.example/callback?action=oauth"}, "resource": {f.p.Resource("api")}, "scope": {"sites:read analytics:read ingest:write"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}, "screen_hint": {"signup"}, "site_domain": {"client.example"}}
	w := httptest.NewRecorder()
	f.p.Authorize(w, httptest.NewRequest(http.MethodGet, f.p.Issuer()+"/oauth/authorize?"+query.Encode(), nil))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != f.p.Issuer()+"/signup?oauth=1" {
		t.Fatalf("signup route: %d %s", w.Code, w.Header().Get("Location"))
	}
	r := httptest.NewRequest(http.MethodGet, f.p.Issuer()+"/api/oauth/consent", nil)
	for _, cookie := range w.Result().Cookies() {
		r.AddCookie(cookie)
	}
	info, err := f.p.Consent(t.Context(), r, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if info.SuggestedDomain != "client.example" {
		t.Fatalf("lost WordPress site hint: %+v", info)
	}
	pending, err := f.p.HasContinuation(t.Context(), r)
	if err != nil || !pending {
		t.Fatalf("lost signup continuation: %v %v", pending, err)
	}
}
