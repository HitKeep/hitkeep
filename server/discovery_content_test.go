package server

import (
	"bytes"
	"encoding/json/v2"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"hitkeep/oauth"
)

func TestPublicDiscoverySitemapAndContentPreferences(t *testing.T) {
	s := newDiscoveryServer()
	rec := discoveryRequest(s, "GET", "/sitemap.xml")
	if rec.Code != http.StatusOK {
		t.Fatalf("sitemap: %d %s", rec.Code, rec.Body.String())
	}
	var sitemap struct {
		XMLName xml.Name `xml:"urlset"`
		URLs    []struct {
			Location string `xml:"loc"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal(rec.Body.Bytes(), &sitemap); err != nil || len(sitemap.URLs) == 0 {
		t.Fatalf("invalid public sitemap: %v %s", err, rec.Body.String())
	}
	for _, item := range sitemap.URLs {
		if !strings.HasPrefix(item.Location, s.conf.PublicURL+"/") || strings.Contains(item.Location, "/dashboard") || strings.Contains(item.Location, "/settings") || strings.Contains(item.Location, "/share/") {
			t.Fatalf("non-public sitemap URL: %s", item.Location)
		}
	}
	robots := discoveryRequest(s, "GET", "/robots.txt").Body.String()
	for _, want := range []string{"Disallow: /", "Sitemap: https://analytics.example.test/tenant/sitemap.xml", "Content-Signal: search=yes, ai-input=yes, ai-train=no"} {
		if !strings.Contains(robots, want) {
			t.Errorf("robots missing %q", want)
		}
	}
	s.conf.AIEnabled = false
	if rec := discoveryRequest(s, "GET", "/sitemap.xml"); rec.Code != http.StatusNotFound {
		t.Fatalf("AI-off sitemap=%d", rec.Code)
	}
	if robots := discoveryRequest(s, "GET", "/robots.txt").Body.String(); strings.Contains(robots, "Sitemap:") || strings.Contains(robots, "Content-Signal:") {
		t.Fatal("AI-off content preferences advertised")
	}
}

func TestRootMarkdownNegotiationIsPublicOnlyAndPrefixBound(t *testing.T) {
	s := newDiscoveryServer()
	s.publicBasePath = "/tenant/"
	handler := s.stripPublicBasePath(s.spaHandler(fstest.MapFS{}))
	for _, tc := range []struct {
		path, accept string
		markdown     bool
	}{
		{"/tenant/", "text/markdown", true},
		{"/tenant", "text/html;q=0.2,text/markdown;q=0.8", true},
		{"/tenant/", "text/markdown;q=0.3,application/json", true},
		{"/tenant/", "text/markdown;q=0,text/html", false},
		{"/tenant/", "text/markdown;q=0.1,text/html;q=0.9", false},
		{"/tenant/", "*/*", false},
		{"/tenant/", "", false},
		{"/tenant/dashboard", "text/markdown", false},
		{"/tenant/api/private", "text/markdown", false},
	} {
		t.Run(tc.path+tc.accept, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, nil)
			req.Header.Set("Accept", tc.accept)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if got := strings.HasPrefix(rec.Header().Get("Content-Type"), "text/markdown"); got != tc.markdown {
				t.Fatalf("markdown=%t want %t: %d %s", got, tc.markdown, rec.Code, rec.Body.String())
			}
			if tc.markdown && (!strings.Contains(rec.Body.String(), s.conf.PublicURL+"/auth.md") || strings.Contains(rec.Body.String(), "dashboard</html>")) {
				t.Fatal("root markdown is not public canonical guidance")
			}
			if tc.path == "/tenant/" && !strings.Contains(rec.Header().Get("Vary"), "Accept") {
				t.Fatal("missing Vary: Accept")
			}
		})
	}
	s.conf.AIEnabled = false
	req := httptest.NewRequest("GET", "/tenant/", nil)
	req.Header.Set("Accept", "text/markdown")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if strings.HasPrefix(rec.Header().Get("Content-Type"), "text/markdown") {
		t.Fatal("AI-off root markdown enabled")
	}
}

func TestRootProtectedResourceAliasKeepsActualAPIResource(t *testing.T) {
	s := newDiscoveryServer()
	p, err := oauth.New(s.ctx.Store, oauth.Options{Issuer: s.conf.PublicURL, MCPPath: s.conf.MCPPath, MCPEnabled: true, BrowserSigningKey: bytes.Repeat([]byte("b"), 32), IsLeader: func() bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	s.ctx.OAuthProvider = p
	alias := discoveryRequest(s, "GET", "/.well-known/oauth-protected-resource")
	expected := httptest.NewRecorder()
	p.ProtectedResourceMetadata("api")(expected, httptest.NewRequest("GET", p.ResourceMetadataURL("api"), nil))
	var actual, original map[string]any
	if err := json.Unmarshal(alias.Body.Bytes(), &actual); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(expected.Body.Bytes(), &original); err != nil {
		t.Fatal(err)
	}
	if alias.Code != http.StatusOK || !reflect.DeepEqual(actual, original) {
		t.Fatalf("alias changed API resource: %d %s", alias.Code, alias.Body.String())
	}
	if alias.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("OAuth cache contract lost")
	}
	s.conf.AIEnabled = false
	if rec := discoveryRequest(s, "GET", "/.well-known/oauth-protected-resource"); rec.Code != http.StatusNotFound {
		t.Fatal("AI-off bootstrap alias enabled")
	}
}

func TestAuthMarkdownDescribesOnlyAvailableHumanClaimFlow(t *testing.T) {
	s := newDiscoveryServer()
	enabled := true
	p, err := oauth.New(s.ctx.Store, oauth.Options{Issuer: s.conf.PublicURL, BrowserSigningKey: bytes.Repeat([]byte("b"), 32), IsLeader: func() bool { return true }, AgentActionsEnabled: func() bool { return enabled }})
	if err != nil {
		t.Fatal(err)
	}
	s.ctx.OAuthProvider = p
	body := discoveryRequest(s, "GET", "/auth.md").Body.String()
	for _, want := range []string{"service_auth", "https://analytics.example.test/tenant/agent/identity", "claim_token", "user_code", "urn:workos:agent-auth:grant-type:claim", "urn:ietf:params:oauth:grant-type:jwt-bearer", "Connected applications"} {
		if !strings.Contains(body, want) {
			t.Errorf("auth.md missing %s", want)
		}
	}
	s.ctx.OAuthProvider = nil
	if body := discoveryRequest(s, "GET", "/auth.md").Body.String(); strings.Contains(body, "Human-approved agent registration") {
		t.Fatal("unavailable registration advertised")
	}
}
