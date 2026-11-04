package system

import "testing"

func TestNativeContractKeepsHumanSessionSeparate(t *testing.T) {
	spec := OpenAPISpecV1("https://example.test/prefix")
	paths := spec["paths"].(map[string]any)
	for _, path := range []string{"/api/auth/native/start", "/api/auth/native/authorize", "/api/auth/native/consent", "/api/auth/native/exchange"} {
		if paths[path] == nil {
			t.Fatal(path)
		}
	}
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)
	exchange := schemas["NativeAuthExchange"].(map[string]any)
	props := exchange["properties"].(map[string]any)
	if len(props) != 4 || exchange["additionalProperties"] != false {
		t.Fatal("exchange fields widened")
	}
	for _, field := range []string{"code", "code_verifier", "client_id", "redirect_uri"} {
		if props[field] == nil {
			t.Fatal(field)
		}
	}
	for _, path := range []string{"/api/auth/native/start", "/api/auth/native/exchange"} {
		operation := paths[path].(map[string]any)["post"].(map[string]any)
		if operation["x-hitkeep-oauth-scope"] != nil || operation["security"] != nil {
			t.Fatal("native exchange became delegated OAuth")
		}
	}
}
