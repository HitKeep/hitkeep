package system

func commerceDiscoveryPaths() map[string]any {
	paths := map[string]any{}
	for _, route := range []string{"/.well-known/acp.json", "/.well-known/ucp"} {
		paths[route] = map[string]any{"get": op([]string{"Cloud"}, "Discover human-approved Cloud checkout", "Public ACP 2026-04-17 or UCP 2026-08-25 profile. Requires AI enabled, the Cloud billing build, hosted mode, configured Stripe Checkout and the authoritative leader/store. Advertises only REST checkout with buyer approval through continue_url; no autonomous payment rail. OAuth billing:write consent or an authorized dashboard browser session is required for checkout actions, not discovery.", nil, nil, nil, map[string]any{"200": jsonSchemaResp("Public checkout profile", map[string]any{"type": "object"}), "404": desc("Commerce unavailable or invalid canonical public URL")})}
	}
	return paths
}

func openAPIV1CommercePaths() map[string]any {
	paths := map[string]any{}
	response := map[string]any{"type": "object", "required": []string{"id", "status", "currency", "line_items", "totals", "messages", "links"}, "properties": map[string]any{
		"id": map[string]any{"type": "string"}, "status": map[string]any{"type": "string", "enum": []string{"requires_escalation", "complete_in_progress", "completed", "canceled", "incomplete"}}, "currency": map[string]any{"type": "string"}, "continue_url": map[string]any{"type": "string", "format": "uri"}, "line_items": map[string]any{"type": "array", "items": map[string]any{"type": "object"}}, "totals": map[string]any{"type": "array", "items": map[string]any{"type": "object"}}, "messages": map[string]any{"type": "array", "items": map[string]any{"type": "object"}}, "links": map[string]any{"type": "array", "items": map[string]any{"type": "object"}}, "order": map[string]any{"type": "object"},
	}}
	for _, protocol := range []string{"acp", "ucp"} {
		base := "/api/cloud/commerce/" + protocol + "/checkout_sessions"
		update := "post"
		version := "2026-04-17"
		if protocol == "ucp" {
			base = "/api/cloud/commerce/ucp/checkout-sessions"
			update = "put"
			version = "2026-08-25"
		}
		for _, route := range []struct{ suffix, method, action string }{{"", "post", "Create"}, {"/{id}", "get", "Retrieve"}, {"/{id}", update, "Update"}, {"/{id}/complete", "post", "Complete"}, {"/{id}/cancel", "post", "Cancel"}} {
			parameters := []any{}
			if route.suffix != "" {
				parameters = append(parameters, map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}})
			}
			if protocol == "acp" {
				parameters = append(parameters, map[string]any{"name": "API-Version", "in": "header", "required": true, "schema": map[string]any{"type": "string", "const": version}})
			}
			if protocol == "ucp" {
				parameters = append(parameters, map[string]any{"name": "UCP-Agent", "in": "header", "required": true, "schema": map[string]any{"type": "string", "description": "profile=\"https://platform.example/.well-known/ucp\"; public HTTPS, exact checkout version, bounded fetch, no redirects"}}, map[string]any{"name": "Request-Id", "in": "header", "required": true, "schema": map[string]any{"type": "string", "minLength": 1, "maxLength": 255}})
			}
			if route.method != "get" {
				parameters = append(parameters, map[string]any{"name": "Idempotency-Key", "in": "header", "required": true, "schema": map[string]any{"type": "string", "minLength": 1, "maxLength": 255}})
			}
			var body any
			if route.method != "get" {
				id := map[string]any{"type": "string", "enum": []string{"pro:monthly", "pro:annual", "business:monthly", "business:annual"}}
				item := map[string]any{"type": "object", "required": []string{"id"}, "properties": map[string]any{"id": id}, "additionalProperties": false}
				if protocol == "ucp" {
					item = map[string]any{"type": "object", "required": []string{"item", "quantity"}, "properties": map[string]any{"item": item, "quantity": map[string]any{"type": "integer", "const": 1}}}
				}
				schema := map[string]any{"type": "object", "properties": map[string]any{"line_items": map[string]any{"type": "array", "minItems": 1, "maxItems": 1, "items": item}}, "description": "One line_items entry: ACP 2026-04-17 uses a direct id and implicitly purchases one subscription; UCP uses item.id and quantity 1. Amounts and regional currency are server-owned. Completed sessions are immutable; changing a plan requires canceling the pending checkout. No payment credential is consumed by this human-approved adapter."}
				if route.action == "Create" {
					schema["required"] = []string{"line_items"}
				}
				body = jsonBody(schema)
			}
			success := "200"
			if route.action == "Create" {
				success = "201"
			}
			operation := cloudOp(route.action+" "+protocol+" checkout", protocol+" "+version+" human-approved session adapter for external assistants or browser users. Requires AI enabled, Cloud billing build, hosted mode, authoritative leader/store and configured Stripe Checkout. External assistants use an API-resource OAuth bearer token with explicit billing:write consent and current team.manage_settings authority on the consented team; billing-only consent does not require a site. Browser sessions retain current active-team billing administration. API-client tokens are unsupported. The consented team remains pinned if the dashboard active team changes, and revocation and current authority are checked before replaying a saved result. Requires escalation through continue_url for buyer approval and payment. Completion is reported only after Stripe retrieves complete and paid; this endpoint does not collect payment credentials or complete payment for the assistant. MPP, x402, AP2 and autonomous spending are not advertised.", append(secCookie(), map[string]any{"oauth2": []string{"billing:write"}}), parameters, body, map[string]any{success: jsonSchemaResp("Authoritative Checkout session", response), "400": desc("Invalid body, version, item or idempotency key"), "401": desc("Valid browser session or user-delegated OAuth token required"), "403": desc("Explicit billing:write consent and current billing administration required"), "404": desc("Disabled feature or session outside the authorized team"), "409": desc("Conflicting checkout, unavailable price, or UCP idempotency mismatch"), "422": desc("ACP idempotency payload mismatch or malformed/incompatible UCP profile"), "424": desc("UCP profile fetch failed"), "405": desc("Checkout cannot be canceled"), "501": desc("Payment rail is not implemented/configured"), "502": desc("Stripe request failed"), "503": desc("Authoritative store, leader or provider unavailable")})
			operation["x-hitkeep-ai-enabled"] = true
			delete(operation, "x-internal")
			path := base + route.suffix
			if paths[path] == nil {
				paths[path] = map[string]any{}
			}
			paths[path].(map[string]any)[route.method] = operation
		}
	}
	return paths
}
