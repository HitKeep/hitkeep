package server

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"hitkeep/config"
	"hitkeep/database"
	json "hitkeep/jsonapi"
	"hitkeep/oauth"
	"hitkeep/server/shared"
	"hitkeep/skills"
)

type discoveryCluster bool

func (c discoveryCluster) IsLeader() bool        { return bool(c) }
func (c discoveryCluster) GetLeaderAddr() string { return "" }

func newDiscoveryServer() *Server {
	conf := &config.Config{PublicURL: "https://analytics.example.test/tenant", MCPPath: "/tools/analytics", MCPEnabled: true, Version: "1.2.3"}
	ctx := &shared.Context{Config: conf, Store: new(database.Store), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return &Server{conf: conf, ctx: ctx, indexHTML: []byte("<html>dashboard</html>")}
}

func discoveryRequest(s *Server, method, route string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	s.registerDiscoveryRoutes(mux)
	req := httptest.NewRequest(method, "https://malicious.example"+route, nil)
	req.Host = "malicious.example"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestDiscoveryRepresentationsAndCanonicalURLs(t *testing.T) {
	s := newDiscoveryServer()
	for route, media := range map[string]string{"/.well-known/api-catalog": apiCatalogMediaType, "/.well-known/ai-catalog.json": "application/ai-catalog+json", "/.well-known/ard.json": "application/json", "/.well-known/integrations.json": "application/json", "/openapi.json": "application/json", "/tools/analytics/server-card": serverCardMediaType, "/.well-known/mcp/server-card.json": "application/json"} {
		t.Run(route, func(t *testing.T) {
			get := discoveryRequest(s, http.MethodGet, route)
			head := discoveryRequest(s, http.MethodHead, route)
			if get.Code != 200 || head.Code != 200 {
				t.Fatalf("GET=%d HEAD=%d %s", get.Code, head.Code, get.Body.String())
			}
			for _, key := range []string{"Content-Type", "Content-Length", "Cache-Control", "Link", "Access-Control-Allow-Origin", "X-Content-Type-Options"} {
				if get.Header().Get(key) != head.Header().Get(key) {
					t.Errorf("HEAD differs for %s", key)
				}
			}
			if get.Header().Get("Content-Type") != media || get.Header().Get("Access-Control-Allow-Origin") != "*" || head.Body.Len() != 0 {
				t.Fatalf("incorrect representation headers %v", get.Header())
			}
			if strings.Contains(get.Body.String(), "malicious.example") || !strings.Contains(get.Body.String(), "https://analytics.example.test/tenant") {
				t.Fatal("metadata is not canonical/prefix-bound")
			}
			if !strings.Contains(get.Header().Get("Link"), `rel="api-catalog"`) {
				t.Fatal("missing catalog Link")
			}
		})
	}
}

func TestDiscoveryCardMatchesMountedMCPAndAuthentication(t *testing.T) {
	s := newDiscoveryServer()
	for _, tc := range []struct {
		name  string
		apply func(*Server)
	}{
		{"disabled", func(s *Server) { s.conf.MCPEnabled = false }},
		{"follower", func(s *Server) { s.ctx.Cluster = discoveryCluster(false) }},
		{"without-store", func(s *Server) { s.ctx.Store = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := newDiscoveryServer()
			tc.apply(next)
			if next.mcpAvailable() {
				t.Fatal("MCP unexpectedly mounted")
			}
			for _, route := range []string{"/tools/analytics/server-card", "/.well-known/mcp/server-card.json"} {
				if got := discoveryRequest(next, "GET", route); got.Code != 404 {
					t.Fatalf("card=%d", got.Code)
				}
			}
			if strings.Contains(discoveryRequest(next, "GET", "/.well-known/ai-catalog.json").Body.String(), serverCardMediaType) {
				t.Fatal("unmounted MCP advertised")
			}
		})
	}
	plain := discoveryRequest(s, "GET", "/tools/analytics/server-card").Body.String()
	if !strings.Contains(plain, "API-client bearer") || strings.Contains(plain, "OAuth access") {
		t.Fatal("token-only description is wrong")
	}
	p, err := oauth.New(s.ctx.Store, oauth.Options{Issuer: s.conf.PublicURL, MCPPath: s.conf.MCPPath, MCPEnabled: true, BrowserSigningKey: bytes.Repeat([]byte("b"), 32), IsLeader: func() bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	s.ctx.OAuthProvider = p
	links := discoveryRequest(s, "GET", "/tools/analytics/server-card").Header().Get("Link")
	if strings.Contains(links, `rel="oauth-authorization-server"`) || !strings.Contains(links, `rel="related"; type="application/json"; title="OAuth authorization server metadata"`) {
		t.Fatal("OAuth metadata must use a registered link relation")
	}
	withOAuth := discoveryRequest(s, "GET", "/tools/analytics/server-card").Body.String()
	if !strings.Contains(withOAuth, "OAuth access") || !strings.Contains(withOAuth, "oauth-protected-resource/tenant/tools/analytics") {
		t.Fatal("OAuth resource metadata absent")
	}
	var card map[string]any
	if err := json.Unmarshal([]byte(withOAuth), &card); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tools", "resources", "prompts", "capabilities", "protocolVersion"} {
		if _, ok := card[key]; ok {
			t.Errorf("obsolete/primitive field %s", key)
		}
	}
	if card["$schema"] != "https://static.modelcontextprotocol.io/schemas/v1/server-card.schema.json" {
		t.Fatal("wrong experimental card schema")
	}
}

func TestDiscoveryRejectsInvalidCanonicalAndUnknownPaths(t *testing.T) {
	for _, base := range []string{"", "/relative", "https://user:secret@analytics.example", "https://analytics.example?secret=x", "https://analytics.example/#private", "https://analytics.example/#", "https://analytics.example/?", "https://:443", "https://analytics.example:0", "https://analytics.example/a/../b", "https://analytics.example/%2fprivate", "https://analytics.example:99999"} {
		t.Run(base, func(t *testing.T) {
			s := newDiscoveryServer()
			s.conf.PublicURL = base
			if got := discoveryRequest(s, "GET", "/.well-known/api-catalog"); got.Code != 404 {
				t.Fatalf("invalid canonical %q status %d", base, got.Code)
			}
		})
	}
	s := newDiscoveryServer()
	for _, route := range []string{"/.well-known/agent-card.json", "/.well-known/ai-catalog.json/unknown", "/tools/analytics/server-card/unknown", "/skills/hitkeep-development/SKILL.md", "/skills/hitkeep-analytics/embed.go", "/skills/hitkeep-analytics/references/other.md"} {
		if got := discoveryRequest(s, "GET", route); got.Code != 404 {
			t.Errorf("%s=%d", route, got.Code)
		}
	}
	if got := discoveryRequest(s, "POST", "/.well-known/api-catalog"); got.Code != 405 {
		t.Errorf("POST=%d", got.Code)
	}
}

func TestDiscoverySkillsAndSeparateCatalogSchemas(t *testing.T) {
	s := newDiscoveryServer()
	for _, skill := range skills.AnalyticsProcedures() {
		for _, suffix := range []string{"SKILL.md", "references/procedure.md"} {
			name := skill.Name + "/" + suffix
			want, err := skills.PublicFile(name)
			if err != nil {
				t.Fatal(err)
			}
			get := discoveryRequest(s, "GET", "/skills/"+name)
			head := discoveryRequest(s, "HEAD", "/skills/"+name)
			if !bytes.Equal(get.Body.Bytes(), want) || head.Body.Len() != 0 || head.Header().Get("Content-Type") != get.Header().Get("Content-Type") {
				t.Errorf("incorrect skill representation %s", name)
			}
		}
	}
	ai := discoveryRequest(s, "GET", "/.well-known/ai-catalog.json")
	ard := discoveryRequest(s, "GET", "/.well-known/ard.json")
	var catalog map[string]any
	var manifest map[string]any
	if err := json.Unmarshal(ai.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(ard.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if catalog["specVersion"] != "1.0" || manifest["specVersion"] != nil {
		t.Fatal("AI Catalog and ARD envelopes conflated")
	}
	entries := manifest["entries"].([]any)
	count := 0
	for _, raw := range entries {
		e := raw.(map[string]any)
		if e["displayName"] == "" || len(e["representativeQueries"].([]any)) < 2 || e["data"] != nil {
			t.Fatal("invalid ARD entry")
		}
		if e["type"] == "application/agent-skills+md" {
			count++
		}
	}
	if count != 5 {
		t.Fatalf("product skill count=%d", count)
	}
}

func TestDiscoveryHomepageLinksKeepDashboardPrivate(t *testing.T) {
	s := newDiscoveryServer()
	rec := httptest.NewRecorder()
	s.spaHandler(os.DirFS(t.TempDir())).ServeHTTP(rec, httptest.NewRequest("GET", "https://malicious.example/", nil))
	if rec.Code != 200 || rec.Header().Get("Link") == "" || strings.Contains(rec.Header().Get("Cache-Control"), "public") {
		t.Fatalf("root headers %v", rec.Header())
	}
	if !s.isNonDashboardPath("/skills/missing") {
		t.Fatal("unknown skill paths must not get dashboard HTML")
	}
}

func TestLegacyDiscoveryCardPreservesConnectionFields(t *testing.T) {
	s := newDiscoveryServer()
	rec := discoveryRequest(s, "GET", "/.well-known/mcp/server-card.json")
	var card map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &card); err != nil {
		t.Fatal(err)
	}
	if card["serverInfo"] == nil || card["authentication"] == nil || card["url"] != "https://analytics.example.test/tenant/tools/analytics" {
		t.Fatalf("legacy connection fields missing: %v", card)
	}
	if card["tools"] != nil || card["resources"] != nil || card["$schema"] != nil {
		t.Fatal("legacy card advertises obsolete schema or primitives")
	}
}

func TestPrefixedDiscoveryRoutingAndPrivateUnknownPaths(t *testing.T) {
	s := newDiscoveryServer()
	s.publicBasePath = "/tenant/"
	mux := http.NewServeMux()
	s.registerDiscoveryRoutes(mux)
	handler := s.stripPublicBasePath(mux)
	for _, route := range []string{"/.well-known/api-catalog", "/tenant/.well-known/api-catalog", "/tenant/tools/analytics/server-card"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", route, nil))
		if rec.Code != 200 {
			t.Errorf("%s=%d", route, rec.Code)
		}
	}
	for _, route := range []string{"/.well-known/unknown", "/tools/analytics/server-card", "/skills/hitkeep-analytics/SKILL.md"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", route, nil))
		if rec.Code != 404 {
			t.Errorf("unprefixed/unknown %s=%d", route, rec.Code)
		}
	}
}

func TestDiscoveryRobotsAllowsOnlyFinitePublicMetadata(t *testing.T) {
	s := newDiscoveryServer()
	rec := discoveryRequest(s, "GET", "/robots.txt")
	body := rec.Body.String()
	for _, want := range []string{"User-agent: *\nDisallow: /\n", "Allow: /tenant/.well-known/api-catalog$", "Allow: /.well-known/api-catalog$", "Allow: /tenant/tools/analytics/server-card$", "Allow: /tenant/skills/hitkeep-analytics/SKILL.md$"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing robots entry %q", want)
		}
	}
	for _, deny := range []string{"Allow: /\n", "Allow: /api/", "/admin/", "/integration/", "hitkeep-development"} {
		if strings.Contains(body, deny) {
			t.Errorf("private route allowance %q", deny)
		}
	}
	s.conf.MCPEnabled = false
	if strings.Contains(discoveryRequest(s, "GET", "/robots.txt").Body.String(), "server-card") {
		t.Fatal("disabled MCP crawl allowance")
	}
	s.conf.PublicURL = ""
	if got := discoveryRequest(s, "GET", "/robots.txt").Body.String(); got != "User-agent: *\nDisallow: /\n" {
		t.Fatalf("unconfigured robots=%q", got)
	}
	if head := discoveryRequest(s, "HEAD", "/robots.txt"); head.Body.Len() != 0 {
		t.Fatal("HEAD robots has a body")
	}
}

func discoveryResponseEntries(t *testing.T, s *Server, route string) []discoveryEntry {
	t.Helper()
	response := discoveryRequest(s, http.MethodGet, route)
	if response.Code != http.StatusOK {
		t.Fatalf("%s returned %d: %s", route, response.Code, response.Body.String())
	}
	var document struct {
		Entries []discoveryEntry `json:"entries"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document.Entries == nil {
		t.Fatalf("%s must contain an entries array, including when empty", route)
	}
	return document.Entries
}

func TestDiscoveryIdentifiersDistinguishCanonicalInstances(t *testing.T) {
	for _, route := range []string{"/.well-known/ai-catalog.json", "/.well-known/ard.json"} {
		t.Run(route, func(t *testing.T) {
			seen := make(map[string]string)
			for _, base := range []string{
				"https://analytics.example.test/one",
				"https://analytics.example.test/two",
				"https://analytics.example.test:8443/one",
				"https://analytics.example.test:9443/one",
				"http://analytics.example.test/one",
			} {
				s := newDiscoveryServer()
				s.conf.PublicURL = base
				entries := discoveryResponseEntries(t, s, route)
				repeated := discoveryResponseEntries(t, s, route)
				if len(entries) != 7 || len(repeated) != len(entries) {
					t.Fatalf("unexpected resource count at %s", base)
				}
				for i, entry := range entries {
					if previous, ok := seen[entry.Identifier]; ok {
						t.Errorf("identifier collision for %s and %s: %s", previous, entry.URL, entry.Identifier)
					}
					seen[entry.Identifier] = entry.URL
					if entry.Identifier != repeated[i].Identifier {
						t.Errorf("unstable identity for %s", entry.URL)
					}
				}
			}
		})
	}
}

func TestDiscoveryPublisherFormatsOnActualMetadata(t *testing.T) {
	// Identifier grammar from the pinned ARD Entry schema; Q validates full serialized manifests separately.
	ardIdentifier := regexp.MustCompile(`^urn:air:[a-zA-Z0-9.-]+(:[a-zA-Z0-9._-]+)+$`)
	for _, tc := range []struct {
		base      string
		publisher string
	}{
		{"https://analytics.example.test:8443/tenant", "analytics.example.test"},
		{"https://ANALYTICS.EXAMPLE.TEST./tenant", "analytics.example.test"},
		{"http://192.0.2.10:8080/tenant", ""},
		{"http://[2001:db8::10]:8080/tenant", ""},
		{"http://localhost:8080/tenant", ""},
		{"http://app.localhost:8080/tenant", ""},
		{"http://analytics.local:8080/tenant", ""},
		{"http://analytics:8080/tenant", ""},
		{"https://-invalid.example/tenant", ""},
	} {
		t.Run(tc.base, func(t *testing.T) {
			s := newDiscoveryServer()
			s.conf.PublicURL = tc.base
			aiEntries := discoveryResponseEntries(t, s, "/.well-known/ai-catalog.json")
			if len(aiEntries) != 7 {
				t.Fatal("AI discovery lost actual resources")
			}
			for _, entry := range aiEntries {
				identifier, err := url.Parse(entry.Identifier)
				if err != nil || !identifier.IsAbs() || entry.Identifier != entry.URL {
					t.Errorf("AI Catalog identifier is not the canonical artifact URI: %q", entry.Identifier)
				}
			}
			ardEntries := discoveryResponseEntries(t, s, "/.well-known/ard.json")
			if tc.publisher == "" {
				if len(ardEntries) != 0 {
					t.Fatal("ARD published resources without a DNS publisher")
				}
			} else {
				if len(ardEntries) != len(aiEntries) {
					t.Fatal("ARD lost resources for a DNS publisher")
				}
				for i, entry := range ardEntries {
					if !ardIdentifier.MatchString(entry.Identifier) || !strings.HasPrefix(entry.Identifier, "urn:air:"+tc.publisher+":") || entry.URL != aiEntries[i].URL {
						t.Errorf("invalid ARD publisher identity: %q", entry.Identifier)
					}
				}
			}
			for _, route := range []string{"/.well-known/api-catalog", "/tools/analytics/server-card", "/.well-known/ard.json"} {
				get := discoveryRequest(s, http.MethodGet, route)
				head := discoveryRequest(s, http.MethodHead, route)
				if get.Code != http.StatusOK || head.Code != get.Code || head.Body.Len() != 0 || head.Header().Get("Content-Type") != get.Header().Get("Content-Type") || get.Header().Get("Access-Control-Allow-Origin") != "*" || !strings.Contains(get.Header().Get("Link"), `rel="api-catalog"`) {
					t.Errorf("publisher handling changed metadata availability or headers at %s", route)
				}
			}
		})
	}
}
