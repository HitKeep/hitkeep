package system

import (
	"maps"

	"github.com/google/jsonschema-go/jsonschema"
	json "hitkeep/jsonapi"
	"testing"
)

func TestNativeManagementOpenAPIRejectsActionAndTargetOverrides(t *testing.T) {
	spec := OpenAPISpecV1("https://example.test/prefix")
	schemas := requireMap(t, requireMap(t, spec, "components"), "schemas")
	encoded, err := json.Marshal(schemas["NativeManagementStart"])
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	base := map[string]any{"client_id": "hitkeep-ios", "redirect_uri": "https://hitkeep.com/auth/native/ios/callback", "code_challenge": "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG", "code_challenge_method": "S256", "state": "random-client-state"}
	for _, tc := range []struct {
		name   string
		fields map[string]any
		valid  bool
	}{
		{"passkey", map[string]any{"action": "passkey_register"}, true},
		{"social", map[string]any{"action": "social_link", "provider": "google"}, true},
		{"search console", map[string]any{"action": "search_console_connect", "team_id": "11111111-2222-3333-4444-555555555555"}, true},
		{"missing provider", map[string]any{"action": "social_link"}, false},
		{"extra provider", map[string]any{"action": "passkey_register", "provider": ""}, false},
		{"extra team", map[string]any{"action": "social_link", "provider": "google", "team_id": "11111111-2222-3333-4444-555555555555"}, false},
		{"actor", map[string]any{"action": "passkey_register", "user_id": "11111111-2222-3333-4444-555555555555"}, false},
		{"purpose", map[string]any{"action": "passkey_register", "purpose": "account_delete"}, false},
		{"generic action", map[string]any{"action": "delete_account"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := make(map[string]any)
			maps.Copy(input, base)
			maps.Copy(input, tc.fields)
			err := resolved.Validate(input)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t want=%t err=%v", err == nil, tc.valid, err)
			}
		})
	}
	paths := requireMap(t, spec, "paths")
	for _, path := range []string{"/api/auth/native/management/start", "/api/auth/native/management/complete"} {
		operation := requireMap(t, requireMap(t, paths, path), "post")
		if operation["security"] == nil || operation["x-hitkeep-oauth-scope"] != nil {
			t.Fatal("management lost ordinary human-cookie boundary")
		}
	}
	complete := requireMap(t, requireMap(t, requireMap(t, requireMap(t, requireMap(t, paths, "/api/auth/native/management/complete"), "post"), "requestBody"), "content"), "application/json")
	if requireMap(t, complete, "schema")["$ref"] != "#/components/schemas/NativeAuthExchange" {
		t.Fatal("management complete changed four-field wire")
	}
}
