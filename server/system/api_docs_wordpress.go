package system

import (
	"context"
	"net/http"
	"strings"

	"hitkeep/server/shared"
)

func addWordPressOpenAPI(components, paths map[string]any) {
	components["securitySchemes"].(map[string]any)["embedBearer"] = map[string]any{"type": "http", "scheme": "bearer", "description": "Dedicated five-minute hk_embed_ credential for one site's read-only analytics and exact parent origin. Not an OAuth access token or dashboard session. Every request rechecks the original consent and current site authority. Requires X-HitKeep-Embed-Parent."}
	parentHeader := map[string]any{"name": "X-HitKeep-Embed-Parent", "in": "header", "required": true, "schema": map[string]any{"type": "string", "format": "uri"}, "description": "Exact parent origin bound to the embed credential."}
	siteParams := make([]any, 1, 2)
	siteParams[0] = paramRef("#/components/parameters/siteID")
	embedSecurity := []any{map[string]any{"embedBearer": []string{}}}
	paths["/api/integrations/sites/{id}/embed"] = map[string]any{"post": op([]string{"Integrations"}, "Create a private analytics embed", "OAuth API-resource analytics:read and explicit current site authority required. Issues a dedicated short-lived read-only bearer and a signed launch URL bound to one site and canonical parent origin. General OAuth tokens remain on the integration server. Parent and iframe exchange the dedicated token using exact-origin postMessage with site and nonce correlation; the browser retains it only in memory. Ordinary private dashboard framing stays forbidden.", []any{map[string]any{"oauth2": []string{"analytics:read"}}}, siteParams,
		jsonBody(map[string]any{"type": "object", "additionalProperties": false, "required": []string{"parent_origin"}, "properties": map[string]any{"parent_origin": map[string]any{"type": "string", "format": "uri", "description": "Canonical HTTPS origin without path/query/fragment; HTTP is limited to loopback development origins."}}}),
		map[string]any{"201": jsonSchemaResp("Private analytics launch", map[string]any{"type": "object", "required": []string{"embed_url", "embed_token", "expires_at", "parent_origin", "site_id"}, "properties": map[string]any{"embed_url": map[string]any{"type": "string", "format": "uri"}, "embed_token": map[string]any{"type": "string"}, "expires_at": map[string]any{"type": "string", "format": "date-time"}, "parent_origin": map[string]any{"type": "string", "format": "uri"}, "site_id": map[string]any{"type": "string", "format": "uuid"}}}), "400": errResp("Invalid parent or site"), "401": errResp("OAuth token required"), "403": errResp("Insufficient scope or site authority"), "503": errResp("Unavailable")})}
	paths["/api/embed/sites/{id}/site"] = map[string]any{"get": op([]string{"Integrations"}, "Read embedded site metadata", "Dedicated embed credential only. Returns sanitized viewer metadata for the one authorized site, without owner identity. Cookies, API keys and ordinary OAuth tokens do not authenticate this endpoint.", embedSecurity, append(siteParams, parentHeader), nil, map[string]any{"200": jsonSchemaResp("Embedded site", map[string]any{"type": "object", "required": []string{"id", "domain", "role"}, "properties": map[string]any{"id": map[string]any{"type": "string", "format": "uuid"}, "domain": map[string]any{"type": "string"}, "role": map[string]any{"const": "viewer"}, "created_at": map[string]any{"type": "string", "format": "date-time"}, "data_retention_days": map[string]any{"type": "integer"}, "retention_synced_from_plan": map[string]any{"type": "boolean"}}}), "401": errResp("Invalid, expired or revoked embed"), "403": errResp("Forbidden"), "503": errResp("Unavailable")})}
	paths["/api/oauth/sites"] = map[string]any{"post": op([]string{"OAuth"}, "Create a site during integration onboarding", "Human account session, browser-bound pending authorization, user-bound CSRF and current active-team owner/admin authority required. Uses normal domain validation, plan quota, retention, audit and webhook policies. Does not grant OAuth consent; the user must still explicitly approve the created site.", secCookie(), nil, jsonBody(map[string]any{"type": "object", "required": []string{"team_id", "csrf_token", "domain"}, "properties": map[string]any{"team_id": map[string]any{"type": "string", "format": "uuid"}, "csrf_token": map[string]any{"type": "string"}, "domain": map[string]any{"type": "string"}}}), map[string]any{"200": jsonRefResp("Created site", "#/components/schemas/Site"), "400": errResp("Invalid domain or CSRF"), "401": errResp("Human session required"), "403": errResp("Team authority or quota denied"), "409": errResp("Site exists or active team changed"), "410": errResp("Pending authorization expired"), "503": errResp("Unavailable")})}
	for _, path := range []string{"/api/sites/{id}/ecommerce", "/api/sites/{id}/ecommerce/timeseries", "/api/sites/{id}/ecommerce/products", "/api/sites/{id}/ecommerce/sources"} {
		get := paths[path].(map[string]any)["get"].(map[string]any)
		get["responses"].(map[string]any)["422"] = errResp("Report contains more than one currency; HitKeep does not combine or convert currencies")
	}
	event := paths["/api/ingest/server/event"].(map[string]any)["post"].(map[string]any)
	event["description"] = event["description"].(string) + " Durable events may omit both visitor_ip and user_agent when original visitor context is unavailable; do not fabricate them. When supplied, both must be valid. Anonymous events retain site, grant, path and DNT checks but cannot apply unknown visitor-IP or user-agent exclusions. event_id supplies stable site-scoped retry deduplication."
	// The same closed policy used by authentication governs which existing reads advertise embed access.
	for path, raw := range paths {
		get, ok := raw.(map[string]any)["get"].(map[string]any)
		if !ok || path == "/api/embed/sites/{id}/site" {
			continue
		}
		r, err := http.NewRequestWithContext(context.Background(), http.MethodGet, strings.NewReplacer("{id}", "00000000-0000-4000-8000-000000000001", "{funnelID}", "00000000-0000-4000-8000-000000000002").Replace(path), nil)
		if err != nil {
			continue
		}
		r.SetPathValue("id", "00000000-0000-4000-8000-000000000001")
		if !shared.EmbedAnalyticsRoute(r) {
			continue
		}
		security, _ := get["security"].([]any)
		get["security"] = append(security, map[string]any{"embedBearer": []string{}})
		get["description"] = get["description"].(string) + " Dedicated site-bound embed credentials may also read this aggregate with X-HitKeep-Embed-Parent; OAuth access tokens follow their separately documented AIEnabled action policy."
	}
	schemas := components["schemas"].(map[string]any)
	schemas["OAuthTeam"].(map[string]any)["properties"].(map[string]any)["can_create_site"] = map[string]any{"type": "boolean", "description": "The active team permits explicit site creation during onboarding."}
	schemas["OAuthConsent"].(map[string]any)["properties"].(map[string]any)["suggested_domain"] = map[string]any{"type": "string", "description": "Untrusted setup hint validated against the registered callback hostname; never grants consent or creates a site automatically."}
}
