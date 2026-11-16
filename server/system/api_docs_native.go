package system

import "maps"

func nativeAuthSchemas() map[string]any {
	text := func() map[string]any { return map[string]any{"type": "string"} }
	closed := func(required []string, properties map[string]any) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties}
	}
	schemas := map[string]any{
		"NativeAuthOK": closed([]string{"status"}, map[string]any{"status": map[string]any{"type": "string", "const": "ok"}}),
		"NativeAuthStart": closed([]string{"client_id", "redirect_uri", "code_challenge", "code_challenge_method", "state", "intent"}, map[string]any{
			"purpose": map[string]any{"type": "string", "const": "account_delete"}, "reauth_attempt": map[string]any{"type": "string", "pattern": "^[A-Za-z0-9_-]{43}$"}, "client_id": map[string]any{"type": "string", "enum": []string{"hitkeep-ios", "hitkeep-android"}}, "redirect_uri": map[string]any{"type": "string", "enum": []string{"https://hitkeep.com/auth/native/ios/callback", "https://hitkeep.com/auth/native/android/callback"}}, "code_challenge": map[string]any{"type": "string", "pattern": "^[A-Za-z0-9_-]{43}$"}, "code_challenge_method": map[string]any{"type": "string", "const": "S256"}, "state": map[string]any{"type": "string", "minLength": 16, "maxLength": 512, "pattern": "^[!-~]+$"}, "intent": map[string]any{"type": "string", "enum": []string{"login", "signup"}}, "remember_me": map[string]any{"type": "boolean"}}),
		"NativeAuthStarted":  closed([]string{"authorization_url", "expires_at"}, map[string]any{"authorization_url": map[string]any{"type": "string", "format": "uri"}, "expires_at": map[string]any{"type": "string", "format": "date-time"}}),
		"NativeAuthExchange": closed([]string{"client_id", "redirect_uri", "code", "code_verifier"}, map[string]any{"client_id": text(), "redirect_uri": text(), "code": map[string]any{"type": "string", "pattern": "^[A-Za-z0-9_-]{43}$"}, "code_verifier": map[string]any{"type": "string", "minLength": 43, "maxLength": 128, "pattern": "^[A-Za-z0-9._~-]+$"}}),
		"NativeAuthStatus": closed([]string{"enabled", "available", "issuer", "start_url", "authorize_url", "exchange_url", "code_challenge_methods_supported", "browser_passkeys", "native_ios_passkeys", "native_android_passkeys"}, map[string]any{
			"enabled": map[string]any{"type": "boolean"}, "available": map[string]any{"type": "boolean"}, "issuer": text(), "start_url": text(), "authorize_url": text(), "exchange_url": text(), "code_challenge_methods_supported": map[string]any{"type": "array", "items": map[string]any{"type": "string", "const": "S256"}}, "browser_passkeys": map[string]any{"type": "boolean"}, "native_ios_passkeys": map[string]any{"type": "boolean", "description": "Requires independently verified RP associated-domain delivery; arbitrary self-hosts use the browser."}, "native_android_passkeys": map[string]any{"type": "boolean", "description": "True only when native authentication is ready and valid explicit Android package and release SHA-256 certificate fingerprints are configured. False by default; this flag does not verify Digital Asset Links deployment or signed-device acceptance."}}),
		"NativeAuthConsent": closed([]string{"request", "client_id", "issuer", "email", "csrf_token", "remember_me", "expires_at"}, map[string]any{"purpose": map[string]any{"type": "string", "const": "account_delete"}, "request": text(), "client_id": text(), "issuer": text(), "email": text(), "csrf_token": text(), "remember_me": map[string]any{"type": "boolean"}, "expires_at": map[string]any{"type": "string", "format": "date-time"}}),
		"NativeAuthError":   closed([]string{"error"}, map[string]any{"error": map[string]any{"type": "string", "enum": []string{"invalid_request", "invalid_grant", "access_denied", "temporarily_unavailable"}}}),
	}
	schemas["NativeManagementStart"] = closed([]string{"client_id", "redirect_uri", "code_challenge", "code_challenge_method", "state", "action"}, map[string]any{
		"client_id": map[string]any{"type": "string", "enum": []string{"hitkeep-ios", "hitkeep-android"}}, "redirect_uri": map[string]any{"type": "string", "enum": []string{"https://hitkeep.com/auth/native/ios/callback", "https://hitkeep.com/auth/native/android/callback"}}, "code_challenge": map[string]any{"type": "string", "pattern": "^[A-Za-z0-9_-]{43}$"}, "code_challenge_method": map[string]any{"type": "string", "const": "S256"}, "state": map[string]any{"type": "string", "minLength": 16, "maxLength": 512, "pattern": "^[!-~]+$"}, "action": map[string]any{"type": "string", "enum": []string{"passkey_register", "social_link", "search_console_connect"}}, "provider": text(), "team_id": map[string]any{"type": "string", "format": "uuid"},
	})
	schemas["NativeManagementStart"].(map[string]any)["oneOf"] = []any{
		map[string]any{"properties": map[string]any{"action": map[string]any{"const": "passkey_register"}}, "not": map[string]any{"anyOf": []any{map[string]any{"required": []string{"provider"}}, map[string]any{"required": []string{"team_id"}}}}},
		map[string]any{"properties": map[string]any{"action": map[string]any{"const": "social_link"}}, "required": []string{"provider"}, "not": map[string]any{"required": []string{"team_id"}}},
		map[string]any{"properties": map[string]any{"action": map[string]any{"const": "search_console_connect"}}, "required": []string{"team_id"}, "not": map[string]any{"required": []string{"provider"}}},
	}
	schemas["NativeManagementStatus"] = closed([]string{"start_url", "complete_url", "actions_supported"}, map[string]any{"start_url": text(), "complete_url": text(), "actions_supported": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"passkey_register", "social_link", "search_console_connect"}}}})
	schemas["NativeAuthStatus"].(map[string]any)["properties"].(map[string]any)["management"] = map[string]any{"$ref": "#/components/schemas/NativeManagementStatus"}
	consentProps := schemas["NativeAuthConsent"].(map[string]any)["properties"].(map[string]any)
	consentProps["purpose"] = map[string]any{"type": "string", "enum": []string{"account_delete", "management"}}
	consentProps["management"] = closed([]string{"action", "phase"}, map[string]any{"action": map[string]any{"type": "string", "enum": []string{"passkey_register", "social_link", "search_console_connect"}}, "provider": text(), "team_id": text(), "team_name": text(), "phase": map[string]any{"type": "string", "enum": []string{"pending", "authorized", "started", "mutating", "succeeded", "failed"}}})
	start := schemas["NativeAuthStart"].(map[string]any)
	start["dependentRequired"] = map[string]any{"purpose": []string{"reauth_attempt"}, "reauth_attempt": []string{"purpose"}}
	start["allOf"] = []any{map[string]any{"if": map[string]any{"required": []string{"purpose"}}, "then": map[string]any{"properties": map[string]any{"intent": map[string]any{"const": "login"}}}}}
	return schemas
}

func nativeAuthPaths() map[string]any {
	jsonResponse := func(description, schema string) map[string]any {
		return jsonRefResp(description, "#/components/schemas/"+schema)
	}
	body := func(schema string) map[string]any {
		return jsonBody(map[string]any{"$ref": "#/components/schemas/" + schema})
	}
	queryParam := func(name, kind string, required bool, description string) map[string]any {
		return map[string]any{"name": name, "in": "query", "required": required, "description": description, "schema": map[string]any{"type": kind}}
	}
	failure := map[string]any{"400": jsonResponse("Invalid or expired request; restart in the app", "NativeAuthError"), "403": jsonResponse("Human cookie, origin or CSRF rejected", "NativeAuthError"), "503": jsonResponse("Authoritative state unavailable; restart if state was lost", "NativeAuthError")}
	response := func(status, schema string) map[string]any {
		out := map[string]any{}
		maps.Copy(out, failure)
		out[status] = jsonResponse("Success", schema)
		return out
	}
	decisionBody := map[string]any{
		"required": true,
		"content": map[string]any{
			"application/x-www-form-urlencoded": map[string]any{
				"schema": map[string]any{
					"type": "object", "additionalProperties": false,
					"required": []string{"request", "csrf_token", "decision"},
					"properties": map[string]any{
						"request":     map[string]any{"type": "string"},
						"csrf_token":  map[string]any{"type": "string"},
						"decision":    map[string]any{"type": "string", "enum": []string{"approve", "deny"}},
						"remember_me": map[string]any{"type": "string", "enum": []string{"true", "false"}},
					},
				},
			},
		},
	}

	description := "First-party full human-session handoff, independent of integration OAuth and AI availability. Public client IDs do not authenticate app binaries. Fixed exact claimed HTTPS callbacks, immutable issuer/prefix/client/PKCE/state bindings, explicit per-attempt full-account consent. All responses no-store; APIs and callback pages no-referrer. Only the exact native consent/manage HTML documents use same-origin referrer policy for strict browser form Origin checks, suppressing cross-origin Referer. No credential-bearing callback or logs. Missing/expired state, consumed codes, issuance failure and lost exchange response require restarting. Native cookies must be isolated per full canonical instance including prefix. Ordinary logout does not instantly revoke copied stateless JWTs."
	paths := map[string]any{
		"/api/auth/native/management/start":    map[string]any{"post": op([]string{"Auth"}, "Start native browser management", "Strict bounded JSON and a current ordinary human JWT only, independent of AI. No remember restoration or cookie issuance. Origin and Referer may be absent; supplied Origin must be exactly the canonical issuer origin. Reject ambient Authorization/API key/embed, duplicate/unknown fields. Freeze actual current user and exactly one action: passkey_register (no target fields), social_link (configured provider only), or search_console_connect (exact authorized team). Fresh same-user primary plus all configured MFA and explicit same-origin browser approval are required before any operation. Uses existing exact HTTPS callbacks and issuer/client/state/S256 bindings; never transfers sessions/provider credentials.", secCookie(), nil, body("NativeManagementStart"), response("200", "NativeAuthStarted"))},
		"/api/auth/native/management/complete": map[string]any{"post": op([]string{"Auth"}, "Complete native management receipt", "Strict four-field NativeAuthExchange JSON; current ordinary native JWT must match the frozen actual user. Missing or expired current identity returns401 without consuming a valid receipt; remember-only credentials cannot restore a session. Rechecks current authority, exact action/provider/team/client/callback/issuer/PKCE, persisted success and expiry; consumes atomically once. Returns status only with NO Set-Cookie or credential. Login/deletion codes are rejected, and management receipts cannot mint login/deletion proof. Lost response requires authoritative security or exact-team GSC refresh, never replay or mutation retry. Origin/Referer may be absent; supplied foreign/null/duplicate Origin is rejected.", secCookie(), nil, body("NativeAuthExchange"), response("200", "NativeAuthOK"))},
		"/api/auth/native/start":               map[string]any{"post": op([]string{"Auth"}, "Start native sign-in", description+" Strict bounded application/json only; S256 required; signup only when Cloud signup enabled. Optional purpose=account_delete and reauth_attempt must occur together with intent=login; they freeze the existing deletion attempt and force actual linked primary plus configured MFA despite existing HitKeep cookies. Approval prepares a separately confirmed deletion and does not delete. Reject duplicate/unknown fields, ambient bearer and disallowed supplied Origin.", nil, nil, body("NativeAuthStart"), response("200", "NativeAuthStarted"))},
		"/api/auth/native/authorize": map[string]any{
			"get":  op([]string{"Auth"}, "Continue native browser sign-in", description+" GET never approves. request is the opaque server handle; resumes ordinary login/signup/provider/MFA.", nil, []any{queryParam("request", "string", true, "Opaque pending request")}, nil, map[string]any{"303": desc("Redirect to same-instance login/signup/consent"), "400": jsonResponse("Expired request", "NativeAuthError")}),
			"post": op([]string{"Auth"}, "Approve or deny native session", description+" Requires current real human cookie, strict same Origin, browser-bound CSRF and final identity/security match. Only one pending-to-approved/denied transition. Redirect contains only opaque code/state/iss or error/state/iss. Remember choice freezes here for login only. Management forbids remember_me: first POST authorizes the exact action and returns to /auth/native/manage; a second explicit browser POST can emit a receipt only after the authoritative operation handler recorded persistence.", secCookie(), nil, decisionBody, map[string]any{"303": desc("Exact claimed HTTPS callback"), "400": jsonResponse("Invalid request", "NativeAuthError"), "403": jsonResponse("Origin, CSRF or identity rejected", "NativeAuthError")})},
		"/api/auth/native/consent":  map[string]any{"get": op([]string{"Auth"}, "Read native session consent", description+" Requires human cookie. Establishes fresh browser-bound CSRF, including after cross-browser email completion; never approves. Management uses the dedicated /auth/native/manage page and returns only its frozen action/provider/team and operation phase.", secCookie(), []any{queryParam("request", "string", true, "Opaque pending request")}, nil, response("200", "NativeAuthConsent"))},
		"/api/auth/native/exchange": map[string]any{"post": op([]string{"Auth"}, "Exchange one-time native code", description+" Strict bounded application/json only. No forms, GET or ambient bearer. Rechecks account, issuer, exact client/callback and S256, atomically consumes before issuing fresh ordinary session/remember cookies; returns status ok. Account-deletion proof binds only to this NEW native cookie JTI, never the intermediate browser session. Never accepts an OAuth grant.", nil, nil, body("NativeAuthExchange"), map[string]any{"200": jsonResponse("Fresh ordinary cookies; no token in body", "NativeAuthOK"), "400": jsonResponse("Invalid grant; restart", "NativeAuthError"), "403": jsonResponse("Ambient bearer rejected", "NativeAuthError"), "503": jsonResponse("Issuance unavailable; restart", "NativeAuthError")})},
	}
	for _, route := range []struct{ path, method string }{
		{"/api/auth/native/management/start", "post"},
		{"/api/auth/native/management/complete", "post"},
		{"/api/auth/native/consent", "get"},
		{"/api/auth/native/authorize", "post"},
	} {
		operation := paths[route.path].(map[string]any)[route.method].(map[string]any)
		operation["responses"].(map[string]any)["401"] = map[string]any{
			"description": "Current human session required; management receipt is not consumed and no cookies are issued",
			"content":     map[string]any{"text/plain": map[string]any{"schema": map[string]any{"type": "string"}}},
		}
	}
	return paths
}
