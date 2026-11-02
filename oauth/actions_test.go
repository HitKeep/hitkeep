package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
)

func TestOAuthActionScopesRequireAIAndNeverReachMCP(t *testing.T) {
	f := newProviderFixture(t)
	enabled := false
	f.options.AgentActionsEnabled = func() bool { return enabled }
	var err error
	f.p, err = New(f.store, f.options)
	if err != nil {
		t.Fatal(err)
	}
	for _, on := range []bool{false, true} {
		enabled = on
		w := httptest.NewRecorder()
		f.p.Metadata(w, httptest.NewRequest("GET", "/metadata", nil))
		var metadata struct {
			Scopes []string `json:"scopes_supported"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &metadata); err != nil {
			t.Fatal(err)
		}
		if slices.Contains(metadata.Scopes, ScopeAnnotationsWrite) != on {
			t.Fatalf("AI=%v scope metadata=%v", on, metadata.Scopes)
		}
		for _, resource := range []string{f.p.Resource("api"), f.p.Resource("mcp")} {
			verifier := strings.Repeat("v", 64)
			challenge := sha256.Sum256([]byte(verifier))
			q := url.Values{"client_id": {"test-client"}, "response_type": {"code"}, "resource": {resource}, "scope": {"sites:read analytics:read annotations:write"}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}}
			w := httptest.NewRecorder()
			f.p.Authorize(w, httptest.NewRequest(http.MethodGet, f.p.Issuer()+"/oauth/authorize?"+q.Encode(), nil))
			location := w.Header().Get("Location")
			valid := on && resource == f.p.Resource("api")
			if (location == f.p.Issuer()+"/oauth/consent") != valid {
				t.Fatalf("AI=%v resource=%s unexpected consent=%s", on, resource, location)
			}
		}
	}
}
