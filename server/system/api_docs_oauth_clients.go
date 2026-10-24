package system

func oauthClientRegistrationProperties() map[string]any {
	return map[string]any{
		"client_name":   map[string]any{"type": "string", "minLength": 1, "maxLength": 128},
		"client_uri":    map[string]any{"type": "string", "description": "Optional public HTTPS application information URL.", "maxLength": 2048},
		"redirect_uris": map[string]any{"type": "array", "minItems": 1, "maxItems": 16, "uniqueItems": true, "items": map[string]any{"type": "string", "format": "uri", "maxLength": 2048}, "description": "Exact HTTPS callbacks without credentials, fragments, wildcards or whitespace."},
	}
}

func openAPIV1OAuthClientPaths() map[string]any {
	team := map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}}
	client := map[string]any{"name": "clientId", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}
	request := map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"client_name", "redirect_uris"}, "properties": oauthClientRegistrationProperties()}}}}
	errors := func() map[string]any {
		return map[string]any{"400": errResp("Invalid public client metadata"), "401": errResp("Human session required"), "403": errResp("Team administration required"), "404": errResp("Client not found in this team"), "409": errResp("Registration capacity or concurrent change"), "503": errResp("Authoritative provider unavailable")}
	}
	list := errors()
	list["200"] = jsonSchemaResp("Team-managed OAuth clients", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/OAuthClientRegistration"}})
	create := errors()
	create["201"] = jsonRefResp("Registered public PKCE client", "#/components/schemas/OAuthClientRegistration")
	update := errors()
	update["200"] = jsonRefResp("Updated public PKCE client", "#/components/schemas/OAuthClientRegistration")
	revoke := errors()
	revoke["204"] = desc("Registration and existing grants revoked.")
	description := "Human session and team.manage_api_clients required (team owner/admin). Registrations are owned by one team and authorize only that team's sites through explicit consent. Public Authorization Code clients use PKCE S256 and no client secret. Operator-configured and HTTPS metadata-document clients are managed separately."
	return map[string]any{
		"/api/user/teams/{id}/oauth-clients": map[string]any{
			"get":  op([]string{"OAuth"}, "List registered OAuth clients", description, secCookie(), []any{team}, nil, list),
			"post": op([]string{"OAuth"}, "Register an OAuth client", description+" Generates a unique immutable client ID; at most 128 active registrations per team. This is dashboard administration, not public dynamic registration.", secCookie(), []any{team}, request, create),
		},
		"/api/user/teams/{id}/oauth-clients/{clientId}": map[string]any{
			"put":    op([]string{"OAuth"}, "Edit an OAuth client", description+" Replaces name, application URL and exact HTTPS redirects. Atomically revokes existing grants and invalidates pending consent from the previous registration version.", secCookie(), []any{team, client}, request, update),
			"delete": op([]string{"OAuth"}, "Revoke an OAuth client", description+" Stops new authorizations and atomically revokes all of the registration's grants. Foreign and nonexistent client IDs are indistinguishable.", secCookie(), []any{team, client}, nil, revoke),
		},
	}
}

func oauthClientRegistrationSchema() map[string]any {
	properties := oauthClientRegistrationProperties()
	properties["client_id"] = map[string]any{"type": "string", "description": "Immutable generated public client ID."}
	return map[string]any{"type": "object", "required": []string{"client_id", "client_name", "redirect_uris"}, "properties": properties}
}
