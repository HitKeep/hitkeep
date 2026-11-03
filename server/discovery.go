package server

import (
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hitkeep/appurl"
	json "hitkeep/jsonapi"
	"hitkeep/mcpserver"
	cloudhandlers "hitkeep/server/cloud"
	"hitkeep/server/shared"
	"hitkeep/server/system"
	"hitkeep/skills"
)

const discoveryCacheControl = "public, max-age=3600"
const apiCatalogMediaType = `application/linkset+json; profile="https://www.rfc-editor.org/info/rfc9727"`
const serverCardMediaType = "application/mcp-server-card+json"

var openAPIDiscoveryPaths = []string{"/openapi.json", "/swagger.json", "/api/openapi.json", "/v1/openapi.json", "/api/schema/{$}"}

// mcpAvailable is the same mount predicate for protocol routing and public metadata.
func (s *Server) mcpAvailable() bool {
	return s.ctx != nil && s.ctx.Config != nil && s.ctx.Config.MCPEnabled && s.ctx.Store != nil && (s.ctx.Cluster == nil || s.ctx.Cluster.IsLeader())
}

func (s *Server) discoveryBase() *url.URL {
	if s.conf == nil {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(s.conf.PublicURL))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(s.conf.PublicURL, "#") || u.RawPath != "" || u.Opaque != "" {
		return nil
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil
		}
	}
	if strings.HasSuffix(u.Host, ":") || strings.ContainsAny(u.Host, "\r\n\t <>{}\\") || strings.ContainsAny(u.Path, "\r\n\t <>{}\\") || strings.TrimSuffix(u.Path, "/") != strings.TrimSuffix(path.Clean("/"+strings.TrimPrefix(u.Path, "/")), "/") {
		return nil
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u
}

func (s *Server) registerDiscoveryRoutes(mux *http.ServeMux) {
	register := func(route string, handler http.HandlerFunc) {
		wrapped := s.ctx.Handler(shared.HandlerConfig{RateLimiter: s.ctx.ApiLimiter}, handler)
		mux.HandleFunc("GET "+route, wrapped)
		if route != "/robots.txt" {
			mux.HandleFunc("OPTIONS "+route, wrapped)
		}
	}
	register("/robots.txt", s.discoveryRobots)
	register("/sitemap.xml", s.agentDiscovery(s.discoverySitemap))
	register("/.well-known/oauth-protected-resource", s.agentDiscovery(func(w http.ResponseWriter, r *http.Request) {
		if s.ctx.OAuthProvider == nil {
			http.NotFound(w, r)
			return
		}
		s.ctx.OAuthProvider.ProtectedResourceMetadata("api")(w, r)
	}))
	for _, route := range []string{"/.well-known/agent-skills/index.json", "/.well-known/skills/index.json"} {
		register(route, s.agentDiscovery(s.discoveryResponse("application/json", func(base *url.URL) any {
			index, err := skills.PublicIndex(base.String())
			if err != nil {
				return nil
			}
			return index
		})))
	}
	register("/.well-known/poppy.json", s.agentDiscovery(s.discoveryResponse("application/json", func(base *url.URL) any {
		if base.Scheme != "https" {
			return nil
		}
		return map[string]any{"protocol_version": "0.2", "organization": map[string]string{"name": "HitKeep", "domain": base.Hostname()}, "web": map[string]any{}}
	})))
	register("/.well-known/agent-card.json", s.agentDiscovery(s.discoveryResponse("application/json", s.a2aCard)))
	register("/.well-known/acp.json", s.discoveryResponse("application/json", s.acpProfile))
	register("/.well-known/ucp", s.discoveryResponse("application/json", s.ucpProfile))
	register("/auth.md", s.agentDiscovery(s.discoveryMarkdown(s.authMarkdown)))
	register("/llms.txt", s.agentDiscovery(s.discoveryMarkdown(s.llmsMarkdown)))
	for _, route := range openAPIDiscoveryPaths {
		register(route, s.discoveryResponse("application/json", func(base *url.URL) any { return system.OpenAPISpecV1ForRuntime(base.String(), s.ctx) }))
	}
	register("/.well-known/api-catalog", s.discoveryResponse(apiCatalogMediaType, s.apiCatalog))
	register("/.well-known/ai-catalog.json", s.discoveryResponse("application/ai-catalog+json", func(base *url.URL) any { return map[string]any{"specVersion": "1.0", "entries": s.agentEntries(base)} }))
	register("/.well-known/ard.json", s.discoveryResponse("application/json", func(base *url.URL) any { return map[string]any{"entries": s.ardEntries(base)} }))
	register("/.well-known/integrations.json", s.discoveryResponse("application/json", s.integrationsDeclaration))
	card := s.discoveryResponse(serverCardMediaType, s.mcpServerCard)
	register(strings.TrimRight(s.conf.MCPPath, "/")+"/server-card", card)
	register("/.well-known/mcp/server-card.json", s.discoveryResponse("application/json", s.legacyMCPServerCard))
	register("/skills/{skill}/{file...}", func(w http.ResponseWriter, r *http.Request) {
		if !s.agentAvailable() || s.discoveryBase() == nil {
			http.NotFound(w, r)
			return
		}
		data, err := skills.PublicFile(r.PathValue("skill") + "/" + r.PathValue("file"))
		if r.PathValue("file") == "skill.zip" {
			data, err = skills.PublicArchive(r.PathValue("skill"))
		}
		if err != nil {
			http.NotFound(w, r)
			return
		}
		media := "text/markdown; charset=utf-8"
		if r.PathValue("file") == "SKILL.md" {
			media = "application/agent-skills+md; charset=utf-8"
		}
		if r.PathValue("file") == "skill.zip" {
			media = "application/zip"
		}
		s.writeDiscovery(w, r, media, data)
	})
}

func (s *Server) discoveryResponse(media string, body func(*url.URL) any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		base := s.discoveryBase()
		if base == nil {
			http.NotFound(w, r)
			return
		}
		value := body(base)
		if value == nil {
			http.NotFound(w, r)
			return
		}
		// Encode before writing headers, and make HEAD carry the same representation metadata.
		data, err := json.Marshal(value)
		if err != nil {
			http.Error(w, "Unable to encode discovery metadata", http.StatusInternalServerError)
			return
		}
		s.writeDiscovery(w, r, media, data)
	}
}

func (s *Server) discoveryHeaders(w http.ResponseWriter, media string) {
	w.Header().Set("Content-Type", media)
	w.Header().Set("Cache-Control", discoveryCacheControl)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, If-None-Match")
	w.Header().Set("Access-Control-Expose-Headers", "ETag, Link")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Link", s.discoveryLinks())
}

func (s *Server) discoveryLinks() string {
	base := s.discoveryBase()
	if base == nil {
		return ""
	}
	links := []string{
		fmt.Sprintf(`<%s>; rel="api-catalog"; type="application/linkset+json"`, appurl.Path(base.String(), "/.well-known/api-catalog")),
		fmt.Sprintf(`<%s>; rel="related"; type="application/ai-catalog+json"`, appurl.Path(base.String(), "/.well-known/ai-catalog.json")),
		fmt.Sprintf(`<%s>; rel="service-desc"; type="application/json"`, appurl.Path(base.String(), "/.well-known/ard.json")),
	}
	if s.agentAvailable() {
		for _, route := range []string{"/.well-known/agent-skills/index.json", "/auth.md", "/llms.txt"} {
			links = append(links, fmt.Sprintf(`<%s>; rel="related"`, appurl.Path(base.String(), route)))
		}
	}
	if s.ctx.A2AAvailable() {
		links = append(links, fmt.Sprintf(`<%s>; rel="related"; type="application/json"; title="A2A agent card"`, appurl.Path(base.String(), "/.well-known/agent-card.json")))
	}
	if s.ctx.OAuthProvider != nil {
		links = append(links, fmt.Sprintf(`<%s>; rel="related"; type="application/json"; title="OAuth authorization server metadata"`, base.Scheme+"://"+base.Host+s.ctx.OAuthProvider.DiscoveryPaths()[0]))
	}
	return strings.Join(links, ", ")
}

func (s *Server) mcpServerCard(base *url.URL) any {
	if !s.mcpAvailable() {
		return nil
	}
	header := "Scoped HitKeep API-client bearer token."
	if s.ctx.OAuthProvider != nil {
		header += " Also accepts an OAuth access token for this MCP resource with sites:read analytics:read and explicitly consented sites."
	}
	return map[string]any{
		"$schema": "https://static.modelcontextprotocol.io/schemas/v1/server-card.schema.json",
		"name":    mcpserver.ServerInfo(s.conf.Version).Name, "version": s.conf.Version,
		"title": mcpserver.ServerInfo(s.conf.Version).Title, "description": mcpserver.ServerInfo(s.conf.Version).Description,
		"websiteUrl": "https://hitkeep.com/guides/integrations/mcp/",
		"remotes":    []any{map[string]any{"type": "streamable-http", "url": appurl.Path(base.String(), s.conf.MCPPath), "supportedProtocolVersions": mcp.SupportedProtocolVersions(), "headers": []any{map[string]any{"name": "Authorization", "description": header, "isRequired": true, "isSecret": true}}}},
		"_meta":      map[string]any{"com.hitkeep/experimental": true, "com.hitkeep/authentication": s.authentication(base)},
	}
}

func (s *Server) authentication(base *url.URL) map[string]any {
	auth := map[string]any{"api_client": "Authorization: Bearer <scoped-api-client-token>", "dashboard_cookies": false}
	if s.ctx.OAuthProvider != nil {
		auth["authorization_server"] = base.Scheme + "://" + base.Host + s.ctx.OAuthProvider.DiscoveryPaths()[0]
		auth["api_resource_metadata"] = s.ctx.OAuthProvider.ResourceMetadataURL("api")
		if s.conf.AIEnabled && s.ctx.OAuthProvider.Ready() {
			auth["delegated_actions"] = map[string]any{"resource": s.ctx.OAuthProvider.Resource("api"), "contract": appurl.Path(base.String(), "/openapi.json"), "scope_field": "x-hitkeep-oauth-scope", "consent": "Explicit team and site consent; billing-only consent does not require sites. Current permissions are checked on every request."}
		}
		if s.mcpAvailable() {
			auth["mcp_resource_metadata"] = s.ctx.OAuthProvider.ResourceMetadataURL("mcp")
		}
	}
	return auth
}

func (s *Server) integrationsDeclaration(base *url.URL) any {
	publicURL := base.String()
	source := appurl.Path(publicURL, "/.well-known/integrations.json")
	credentialID := "hitkeep_api_client_token" // #nosec G101 -- manifest identifier naming the credential type, not a secret.
	surfaces := []map[string]any{
		s.restSurface(publicURL, source, credentialID),
	}
	if s.mcpAvailable() {
		surfaces = append(surfaces, s.mcpSurface(publicURL, source, credentialID))
	}
	return map[string]any{
		"version":        3,
		"summary":        "HitKeep exposes a REST API and optional read-only MCP server for privacy-first analytics.",
		"authentication": s.authentication(base),
		"credentials": map[string]any{
			credentialID: map[string]any{
				"type":        "bearer",
				"label":       "HitKeep API client token",
				"generateUrl": appurl.Path(publicURL, "/integration/api-clients"),
				"setup":       "In the HitKeep app, open Integration > API clients and create a personal or team API client. Send the issued token as an Authorization Bearer token.",
			},
		},
		"surfaces": surfaces,
	}
}

func (s *Server) restSurface(publicURL, source, credentialID string) map[string]any {
	return map[string]any{
		"slug": "hitkeep-rest-api",
		"name": "HitKeep REST API",
		"type": "http",
		"docs": "https://hitkeep.com/api/",
		"spec": appurl.Path(publicURL, "/openapi.json"),
		"url":  appurl.Path(publicURL, "/api"),
		"basis": map[string]any{
			"via":    "declared",
			"source": source,
		},
		"auth": requiredBearerAuth(source, credentialID),
	}
}

func (s *Server) mcpSurface(publicURL, source, credentialID string) map[string]any {
	return map[string]any{
		"slug":       "hitkeep-mcp-server",
		"name":       "HitKeep MCP server",
		"type":       "mcp",
		"docs":       "https://hitkeep.com/use-cases/read-only-mcp-server-web-analytics/",
		"url":        appurl.Path(publicURL, s.conf.MCPPath),
		"transports": []string{"streamable-http"},
		"basis": map[string]any{
			"via":    "declared",
			"source": source,
		},
		"auth": requiredBearerAuth(source, credentialID),
	}
}

func requiredBearerAuth(source, credentialID string) map[string]any {
	basis := map[string]any{
		"via":    "declared",
		"source": source,
	}
	return map[string]any{
		"status": "required",
		"entries": []map[string]any{
			{
				"use": []map[string]any{
					{
						"id": credentialID,
						"mechanics": map[string]any{
							"source":     "http",
							"in":         "header",
							"headerName": "Authorization",
							"scheme":     "Bearer",
						},
					},
				},
				"basis": basis,
			},
		},
	}
}

func (s *Server) apiCatalog(base *url.URL) any {
	linkset := []any{map[string]any{"anchor": appurl.Path(base.String(), "/api"), "service-desc": []any{map[string]string{"href": appurl.Path(base.String(), "/openapi.json"), "type": "application/json"}}, "service-doc": []any{map[string]string{"href": "https://hitkeep.com/api/", "type": "text/html"}}}}
	if s.mcpAvailable() {
		linkset = append(linkset, map[string]any{"anchor": appurl.Path(base.String(), s.conf.MCPPath), "service-desc": []any{map[string]string{"href": appurl.Path(base.String(), strings.TrimRight(s.conf.MCPPath, "/")+"/server-card"), "type": serverCardMediaType}}})
	}
	if cloudhandlers.PlansAvailable(s.conf) {
		linkset = append(linkset, map[string]any{"anchor": appurl.Path(base.String(), "/api/cloud/plans"), "service-doc": []any{map[string]string{"href": "https://hitkeep.com/pricing/", "type": "text/html"}}})
	}
	if s.ctx.A2AAvailable() {
		linkset = append(linkset, map[string]any{"anchor": appurl.Path(base.String(), "/api/a2a"), "service-desc": []any{map[string]string{"href": appurl.Path(base.String(), "/.well-known/agent-card.json"), "type": "application/json"}}})
	}
	return map[string]any{"linkset": linkset}
}

type discoveryEntry struct {
	Identifier            string   `json:"identifier"`
	DisplayName           string   `json:"displayName"`
	Type                  string   `json:"type"`
	URL                   string   `json:"url"`
	Description           string   `json:"description"`
	RepresentativeQueries []string `json:"representativeQueries"`
}

func (s *Server) agentEntries(base *url.URL) []discoveryEntry {
	entries := []discoveryEntry{{"", "HitKeep REST API", "application/vnd.oai.openapi+json", appurl.Path(base.String(), "/openapi.json"), "HTTP API with scoped analytics and explicitly consented OAuth product actions where enabled.", []string{"How can I read approved HitKeep analytics?", "Where is the HitKeep API authentication contract?"}}}
	if s.mcpAvailable() {
		entries = append(entries, discoveryEntry{"", "HitKeep Analytics MCP", serverCardMediaType, appurl.Path(base.String(), strings.TrimRight(s.conf.MCPPath, "/")+"/server-card"), "Read-only aggregate analytics; server card discovery is experimental.", []string{"How can my assistant read approved analytics?", "Where can I connect a scoped HitKeep MCP client?"}})
	}
	if s.agentAvailable() {
		for _, skill := range skills.AnalyticsProcedures() {
			entries = append(entries, discoveryEntry{"", skill.Title, "application/agent-skills+md", appurl.Path(base.String(), "/skills/"+skill.Name+"/SKILL.md"), "Official product analytics instructions; installation does not grant data access.", []string{"How do I use " + skill.Name + " with approved analytics?", "Where are the " + skill.Name + " procedure and privacy boundaries?"}})
		}
	}
	if s.ctx.A2AAvailable() {
		entries = append(entries, discoveryEntry{"", "HitKeep synchronous analytics agent", "application/a2a+json", appurl.Path(base.String(), "/.well-known/agent-card.json"), "A2A 1.0 structured read-only analytics using API-resource OAuth, current site permissions and explicit consent.", []string{"How can an agent read bounded aggregate analytics?", "Which analytics tools accept structured A2A messages?"}})
	}
	if s.commerceAvailable() {
		entries = append(entries, discoveryEntry{"", "HitKeep Cloud checkout", "application/vnd.oai.openapi+json", appurl.Path(base.String(), "/openapi.json") + "#/paths/~1api~1cloud~1commerce~1acp~1checkout_sessions", "ACP/UCP checkout adapters require explicit billing:write OAuth consent or an authenticated dashboard browser session, current billing administration and human Stripe Checkout approval. No autonomous payment rail.", []string{"How can a billing administrator open hosted checkout?", "How does checkout require human payment approval?"}})
	}
	for i := range entries {
		entries[i].Identifier = entries[i].URL
	}
	return entries
}

func (s *Server) ardEntries(base *url.URL) []discoveryEntry {
	publisher := strings.ToLower(strings.TrimSuffix(base.Hostname(), "."))
	// ARD requires a DNS publisher; AI Catalog URI identifiers still support local/IP instances.
	if _, err := netip.ParseAddr(publisher); err == nil || !strings.Contains(publisher, ".") || len(publisher) > 253 || strings.HasSuffix(publisher, ".localhost") || strings.HasSuffix(publisher, ".local") {
		return []discoveryEntry{}
	}
	for label := range strings.SplitSeq(publisher, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return []discoveryEntry{}
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-') {
				return []discoveryEntry{}
			}
		}
	}
	entries := s.agentEntries(base)
	for i := range entries {
		// A bounded resource digest distinguishes configured schemes, ports and prefixes without registration state.
		entries[i].Identifier = fmt.Sprintf("urn:air:%s:resource:%x", publisher, sha256.Sum256([]byte(entries[i].URL)))
	}
	return entries
}

// Discovery well-known locations remain reachable at the origin for prefixed instances.
func (s *Server) isDiscoveryRoot(requestPath string) bool {
	switch requestPath {
	case "/.well-known/acp.json", "/.well-known/ucp", "/.well-known/agent-card.json", "/auth.md", "/llms.txt", "/.well-known/agent-skills/index.json", "/.well-known/skills/index.json", "/.well-known/poppy.json", "/robots.txt", "/sitemap.xml", "/.well-known/oauth-protected-resource", "/.well-known/api-catalog", "/.well-known/ai-catalog.json", "/.well-known/ard.json", "/.well-known/integrations.json", "/.well-known/mcp/server-card.json":
		return true
	default:
		return false
	}
}

// The legacy URL retains connection fields for existing discovery clients.
func (s *Server) legacyMCPServerCard(base *url.URL) any {
	if !s.mcpAvailable() {
		return nil
	}
	return map[string]any{
		"version": "1.0", "protocolVersion": mcp.SupportedProtocolVersions()[0],
		"url": appurl.Path(base.String(), s.conf.MCPPath), "serverInfo": mcpserver.ServerInfo(s.conf.Version),
		"description":      mcpserver.ServerInfo(s.conf.Version).Description,
		"documentationUrl": "https://hitkeep.com/guides/integrations/mcp/",
		"transport":        map[string]string{"type": "streamable-http", "endpoint": s.conf.MCPPath},
		"authentication":   map[string]any{"type": "bearer", "required": true, "schemes": []string{"bearer"}, "details": s.authentication(base)},
	}
}

func (s *Server) discoveryRobots(w http.ResponseWriter, r *http.Request) {
	var text strings.Builder
	text.WriteString("User-agent: *\nDisallow: /\n")
	if base := s.discoveryBase(); base != nil {
		allow := func(route string) { text.WriteString("Allow: " + base.Path + route + "$\n") }
		for _, route := range []string{"/.well-known/api-catalog", "/.well-known/ai-catalog.json", "/.well-known/ard.json", "/.well-known/integrations.json"} {
			allow(route)
			if base.Path != "" {
				text.WriteString("Allow: " + route + "$\n")
			}
		}
		for _, route := range openAPIDiscoveryPaths {
			allow(strings.TrimSuffix(route, "{$}"))
		}
		if s.mcpAvailable() {
			allow(strings.TrimRight(s.conf.MCPPath, "/") + "/server-card")
			allow("/.well-known/mcp/server-card.json")
			if base.Path != "" {
				text.WriteString("Allow: /.well-known/mcp/server-card.json$\n")
			}
		}
		if s.agentAvailable() {
			text.WriteString("Content-Signal: search=yes, ai-input=yes, ai-train=no\n")
			text.WriteString("Sitemap: " + appurl.Path(base.String(), "/sitemap.xml") + "\n")
			allow("/sitemap.xml")
			if s.ctx.OAuthProvider != nil {
				allow("/.well-known/oauth-protected-resource")
				if base.Path != "" {
					text.WriteString("Allow: /.well-known/oauth-protected-resource$\n")
				}
			}
			for _, route := range []string{"/.well-known/agent-skills/index.json", "/.well-known/skills/index.json", "/auth.md", "/llms.txt"} {
				allow(route)
				if base.Path != "" {
					text.WriteString("Allow: " + route + "$\n")
				}
			}
			if base.Scheme == "https" {
				allow("/.well-known/poppy.json")
				if base.Path != "" {
					text.WriteString("Allow: /.well-known/poppy.json$\n")
				}
			}
			for _, skill := range skills.AnalyticsProcedures() {
				allow("/skills/" + skill.Name + "/skill.zip")
				allow("/skills/" + skill.Name + "/SKILL.md")
				allow("/skills/" + skill.Name + "/references/procedure.md")
			}
		}
		if s.ctx.A2AAvailable() {
			allow("/.well-known/agent-card.json")
			if base.Path != "" {
				text.WriteString("Allow: /.well-known/agent-card.json$\n")
			}
		}
		if s.ctx.OAuthProvider != nil {
			for _, route := range s.ctx.OAuthProvider.DiscoveryPaths() {
				text.WriteString("Allow: " + route + "$\n")
			}
		}
		if cloudhandlers.PlansAvailable(s.conf) {
			allow("/api/cloud/plans")
		}
	}
	data := []byte(text.String())
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", discoveryCacheControl)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func (s *Server) agentAvailable() bool { return s.conf != nil && s.conf.AIEnabled }

func (s *Server) discoverySitemap(w http.ResponseWriter, r *http.Request) {
	base := s.discoveryBase()
	if base == nil {
		http.NotFound(w, r)
		return
	}
	type entry struct {
		Location string `xml:"loc"`
	}
	document := struct {
		XMLName xml.Name `xml:"urlset"`
		XMLNS   string   `xml:"xmlns,attr"`
		URLs    []entry  `xml:"url"`
	}{XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	// Only public integration documents belong here, never dashboard or shared analytics URLs.
	for _, route := range []string{"/openapi.json", "/auth.md", "/llms.txt", "/.well-known/api-catalog", "/.well-known/agent-skills/index.json"} {
		document.URLs = append(document.URLs, entry{Location: appurl.Path(base.String(), route)})
	}
	data, err := xml.Marshal(document)
	if err != nil {
		http.Error(w, "Unable to encode sitemap", http.StatusInternalServerError)
		return
	}
	s.writeDiscovery(w, r, "application/xml; charset=utf-8", append([]byte(xml.Header), data...))
}

func prefersPublicMarkdown(accept string) bool {
	var markdown, html float64
	htmlSpecificity := -1
	for item := range strings.SplitSeq(accept, ",") {
		media, parameters, err := mime.ParseMediaType(strings.TrimSpace(item))
		if err != nil {
			continue
		}
		quality := 1.0
		if raw, ok := parameters["q"]; ok {
			quality, err = strconv.ParseFloat(raw, 64)
			if err != nil || !(quality >= 0 && quality <= 1) {
				continue
			}
		}
		if media == "text/markdown" {
			markdown = max(markdown, quality)
			continue
		}
		specificity := -1
		switch media {
		case "text/html":
			specificity = 2
		case "text/*":
			specificity = 1
		case "*/*":
			specificity = 0
		}
		if specificity < 0 {
			continue
		}
		if specificity > htmlSpecificity {
			html, htmlSpecificity = quality, specificity
		} else if specificity == htmlSpecificity {
			html = max(html, quality)
		}
	}
	return markdown > 0 && markdown >= html
}

func (s *Server) agentDiscovery(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.agentAvailable() {
			http.NotFound(w, r)
			return
		}
		next(w, r)
	}
}

func (s *Server) writeDiscovery(w http.ResponseWriter, r *http.Request, media string, data []byte) {
	s.discoveryHeaders(w, media)
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(data))
	w.Header().Set("ETag", etag)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	for tag := range strings.SplitSeq(r.Header.Get("If-None-Match"), ",") {
		tag = strings.TrimPrefix(strings.TrimSpace(tag), "W/")
		if tag == etag || tag == "*" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if r.Method != http.MethodHead {
		// #nosec G705 -- Only embedded public artifacts or server-owned discovery text; non-HTML media and nosniff are set above.
		_, _ = w.Write(data)
	}
}

func (s *Server) discoveryMarkdown(body func(*url.URL) string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		base := s.discoveryBase()
		if base == nil {
			http.NotFound(w, r)
			return
		}
		s.writeDiscovery(w, r, "text/markdown; charset=utf-8", []byte(body(base)))
	}
}

func (s *Server) authMarkdown(base *url.URL) string {
	text := "# Auth.md: HitKeep authentication\n\nUse a scoped API-client bearer token created by the account owner at " + appurl.Path(base.String(), "/integration/api-clients") + ". Send it in the Authorization: Bearer header. Never send dashboard cookies to MCP or ask for passwords.\n\n"
	if s.ctx.OAuthProvider != nil {
		text += "For OAuth, discover authorization server metadata at " + base.Scheme + "://" + base.Host + s.ctx.OAuthProvider.DiscoveryPaths()[0] + ". Use the existing authorization code flow with PKCE S256, a registered client ID or HTTPS client metadata document, the exact resource and least required scopes. The user signs in and explicitly consents to sites. Do not reuse tokens across resources. API resource metadata: " + s.ctx.OAuthProvider.ResourceMetadataURL("api") + ".\n\n"
		if s.conf.AIEnabled && s.ctx.OAuthProvider.Ready() {
			text += s.agentAuthMarkdown()
		}
		text += "External assistants can use user-delegated OAuth at ordinary REST operations marked x-hitkeep-oauth-scope in OpenAPI while AI is enabled. Request only the operation's scope: sites:read or analytics:read for approved reads; goals:write for goals/funnels; annotations:write for timeline notes; sites:write for domain, retention and exclusions; opportunities:write for generation/status; billing:write for configured team Checkout. Each request intersects the grant with current user, team and site permissions. Billing-only consent selects a team without requiring sites; payment still needs human approval. No scopes grant credential issuance, impersonation, raw export or instance administration. Review and revoke grants in Settings → Connected applications.\n\n"
	}
	if s.mcpAvailable() {
		text += "MCP endpoint: " + appurl.Path(base.String(), s.conf.MCPPath) + ". MCP accepts scoped bearer tokens for read-only aggregate analytics; OAuth requires sites:read and analytics:read plus explicitly consented sites.\n\n"
	}
	if s.ctx.A2AAvailable() {
		text += "A2A 1.0 synchronous analytics: " + appurl.Path(base.String(), "/api/a2a/message:send") + ". Use the API OAuth resource with sites:read and analytics:read. Discover tool inputs at " + appurl.Path(base.String(), "/.well-known/agent-card.json") + ".\n\n"
	}
	return text + "API contract: " + appurl.Path(base.String(), "/openapi.json") + ". Skill installation and discovery grant no account or site access. No anonymous agent registration or Poppy session authentication is provided.\n"
}

// This walkthrough describes only the implemented human-claimed service_auth method.
func (s *Server) agentAuthMarkdown() string {
	issuer := s.ctx.OAuthProvider.Issuer()
	resource := s.ctx.OAuthProvider.Resource("api")
	return fmt.Sprintf(`## Human-approved agent registration

HitKeep supports Auth.md user-claimed service_auth while AI is enabled. An assistant cannot create an account or access data before a human approves. Anonymous and provider identity-assertion registration are unsupported. Discover the actual endpoints and supported methods in authorization server metadata; identity_endpoint/register_uri and claim_endpoint/claim_uri are compatibility aliases for the same implemented routes.

1. Ask the user for their existing HitKeep account email and the access they want. Register using the discovered identity_endpoint:

~~~http
POST %s/agent/identity
Content-Type: application/json
~~~
~~~json
{"type":"service_auth","login_hint":"user@example.com","resource":"%s","scope":"sites:read analytics:read"}
~~~

   For Auth.md v0.5 clients, the same endpoint also accepts {"type":"identity_assertion","assertion_type":"verified_email","assertion":"user@example.com"} and returns registration_type=email-verification with the same claim ceremony. This compatibility name is only an email hint: it accepts no provider JWT or proof and grants no access. The current service_auth form is preferred. Both forms require the identical native human approval below.

   The response contains registration_id, claim_token, claim_token_expires, post_claim_scopes, and claim with user_code, verification_uri, expires_in, interval. It contains no access_token or identity_assertion. Save claim_token privately; never put it in a URL or log.
2. Show the user the six-digit claim.user_code and claim.verification_uri together. The user opens that link, signs in through HitKeep's normal sign-in/MFA/SSO, types the code on HitKeep, selects the team and sites, reviews every scope, and approves or denies. Their signed-in account email must match login_hint. Never ask the user to send their password, session cookie, or confirmation code back to the assistant. The link alone cannot approve access.
3. Poll %s/oauth/token with Content-Type: application/x-www-form-urlencoded and grant_type=urn:workos:agent-auth:grant-type:claim plus claim_token. Wait at least claim.interval seconds between polls. authorization_pending means keep waiting; slow_down means add five seconds to the interval. Stop on access_denied. Codes expire within ten minutes; the outer claim window is fifteen minutes, with at most five incorrect code submissions. Before the outer window expires, POST JSON {"claim_token":"<saved handle>","email":"user@example.com"} to %s/agent/identity/claim to replace a code and invalidate the old link; use claim_attempt from that response. After outer expiry or lockout, start a new registration.
4. After approval, one successful poll returns access_token, token_type=Bearer, expires_in, resource, scope, identity_assertion and assertion_expires. The claim handle is consumed once. Send the access token only to its resource in the Authorization: Bearer header. Access tokens last at most five minutes.
5. Until assertion_expires, refresh by POSTing form fields grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer, assertion=<identity_assertion>, and resource=%s to %s/oauth/token. No refresh_token is issued. The same service-signed assertion is intentionally reusable for this registration; it expires no later than one hour after approval. Every exchange rechecks current user, team, site and scope authority. The assertion cannot switch resources or expand scopes. Start a new human-approved registration after expiry.
6. To revoke one agent access token, POST token=<access_token>&token_type_hint=access_token to %s/oauth/revoke. That assertion remains usable until expiry. To disconnect the registration and revoke all its tokens and assertion refreshes, the human revokes its entry in Settings → Connected applications. While AI is disabled, agent registration and its credentials are disabled. Ordinary OAuth code clients keep their existing refresh rotation and credential-family revocation.

Use least privilege. For MCP, request its distinct configured resource with sites:read and analytics:read; MCP remains read-only. API scopes and operations are listed below and in OpenAPI. Billing requires explicit billing:write team consent and a separate human payment handoff; registration never performs a payment.

Public service information: [pricing](https://hitkeep.com/pricing), [terms](https://hitkeep.com/terms), [privacy](https://hitkeep.com/privacy). Integration support: https://github.com/HitKeep/hitkeep/issues. Self-hosted operators may set their own terms and pricing.

`, issuer, resource, issuer, issuer, resource, issuer, issuer)
}

func (s *Server) llmsMarkdown(base *url.URL) string {
	text := "# HitKeep\n\n> Privacy-first web analytics. This application requires approved account and site access.\n\n## Integration references\n\n- [API contract](" + appurl.Path(base.String(), "/openapi.json") + ")\n- [Authentication](" + appurl.Path(base.String(), "/auth.md") + ")\n- [Product analytics skills](" + appurl.Path(base.String(), "/.well-known/agent-skills/index.json") + ")\n- [Public documentation](https://hitkeep.com/llms.txt)\n"
	if s.mcpAvailable() {
		text += "- [Read-only MCP server](" + appurl.Path(base.String(), strings.TrimRight(s.conf.MCPPath, "/")+"/server-card") + ")\n"
	}
	return text
}
