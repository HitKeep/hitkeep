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

func TestDeletionAuthenticationContractIsNarrow(t *testing.T) {
	spec := OpenAPISpecV1("https://example.test/prefix")
	paths := spec["paths"].(map[string]any)
	if paths["/api/auth/mfa/resume"] == nil {
		t.Fatal("missing verified-primary MFA resume")
	}
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)
	start := schemas["NativeAuthStart"].(map[string]any)
	props := start["properties"].(map[string]any)
	if props["purpose"].(map[string]any)["const"] != "account_delete" || start["dependentRequired"] == nil || start["allOf"] == nil {
		t.Fatal("purpose/attempt pairing missing")
	}
	exchange := schemas["NativeAuthExchange"].(map[string]any)
	if len(exchange["properties"].(map[string]any)) != 4 {
		t.Fatal("native exchange changed")
	}
}

func TestNativeHumanSessionContractDocumentsPlainTextUnauthorized(t *testing.T) {
	paths := nativeAuthPaths()
	for _, route := range []struct{ path, method string }{
		{"/api/auth/native/management/start", "post"},
		{"/api/auth/native/management/complete", "post"},
		{"/api/auth/native/consent", "get"},
		{"/api/auth/native/authorize", "post"},
	} {
		operation := paths[route.path].(map[string]any)[route.method].(map[string]any)
		response, ok := operation["responses"].(map[string]any)["401"].(map[string]any)
		if !ok {
			t.Errorf("%s %s omits unauthenticated response", route.method, route.path)
			continue
		}
		content, ok := response["content"].(map[string]any)["text/plain"].(map[string]any)
		if !ok || content["schema"].(map[string]any)["type"] != "string" {
			t.Errorf("%s misrepresents plain-text 401", route.path)
		}
	}
}
