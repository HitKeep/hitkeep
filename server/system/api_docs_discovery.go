package system

// agentDiscoveryPaths describes optional AI-enabled public discovery and analytics.
// Runtime callers include these paths only when the instance AI feature flag is on.
func agentDiscoveryPaths() map[string]any {
	paths := map[string]any{}
	for route, media := range map[string]string{
		"/.well-known/agent-skills/index.json": "application/json",
		"/.well-known/skills/index.json":       "application/json",
		"/.well-known/poppy.json":              "application/json",
		"/.well-known/agent-card.json":         "application/json",
		"/auth.md":                             "text/markdown",
		"/llms.txt":                            "text/markdown",
	} {
		kind := "string"
		if media == "application/json" {
			kind = "object"
		}
		paths[route] = map[string]any{"get": op([]string{"Integrations"}, "Read agent discovery", "Requires AIEnabled and a valid canonical public URL. Poppy is website-only version 0.2. A2A card additionally requires ready OAuth, leader, control store and tenant analytics routing. Discovery grants no account access.", nil, nil, nil, map[string]any{"200": map[string]any{"description": "Public discovery representation", "content": map[string]any{media: map[string]any{"schema": map[string]any{"type": kind}}}}, "404": desc("Feature or representation unavailable")})}
	}
	paths["/skills/{skill}/skill.zip"] = map[string]any{"get": op([]string{"Integrations"}, "Download canonical product skill", "Requires AIEnabled. Deterministic ZIP containing root SKILL.md and references/procedure.md only. v0.2 index digest is SHA-256 of these raw bytes.", nil, []any{map[string]any{"name": "skill", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}}, nil, map[string]any{"200": map[string]any{"description": "Skill archive", "content": map[string]any{"application/zip": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}}}, "404": desc("Unknown skill or disabled feature")})}
	paths["/api/a2a/message:send"] = map[string]any{"post": op([]string{"Integrations"}, "Read synchronous A2A analytics", "A2A 1.0 HTTP+JSON. Send one ROLE_USER message containing one data part {tool, arguments}; tool names and input schemas are in the agent card. Executes the existing read-only aggregate registry with 31-day range, 64 KiB request and 15-second execution bounds. Requires AIEnabled, ready OAuth, both sites:read and analytics:read for the API resource, current site permission and explicit consent. API-client tokens and dashboard cookies are unsupported. Returns a direct ROLE_AGENT message, no persisted tasks, model calls, streaming or push notifications.", []any{map[string]any{"oauth2": []string{"sites:read", "analytics:read"}}}, []any{map[string]any{"name": "A2A-Version", "in": "header", "required": true, "schema": map[string]any{"type": "string", "const": "1.0"}}}, map[string]any{"required": true, "content": map[string]any{"application/a2a+json": map[string]any{"schema": map[string]any{"type": "object"}, "example": map[string]any{"message": map[string]any{"messageId": "request-1", "role": "ROLE_USER", "parts": []any{map[string]any{"data": map[string]any{"tool": "hitkeep_get_site_overview", "arguments": map[string]any{"site_id": "00000000-0000-4000-8000-000000000001", "sections": []string{"kpis"}}}}}}}}}}, map[string]any{"200": desc("Direct structured analytics message"), "400": desc("Invalid message, unsupported version or configuration"), "401": desc("OAuth authentication required"), "403": desc("Consent, scope or site permission denied"), "404": desc("Unavailable feature or task")})}
	return paths
}
