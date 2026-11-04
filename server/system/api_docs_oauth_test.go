package system

import (
	"slices"
	"testing"
)

func TestOpenAPIOAuthKeepsBrowserAndIntegrationAuthenticationSeparate(t *testing.T) {
	spec := OpenAPISpecV1("https://analytics.example/tenant")
	paths := spec["paths"].(map[string]any)
	for _, test := range []struct{ path, scheme string }{
		{"/api/oauth/consent", "cookieAuth"},
		{"/api/user/oauth-grants", "cookieAuth"},
		{"/api/integrations/sites", "oauth2"},
		{"/api/integrations/sites/{id}/preview", "oauth2"},
	} {
		operation := paths[test.path].(map[string]any)["get"].(map[string]any)
		security := operation["security"].([]any)
		if len(security) != 1 {
			t.Fatalf("%s advertises alternative authentication: %#v", test.path, security)
		}
		if _, ok := security[0].(map[string]any)[test.scheme]; !ok {
			t.Errorf("%s does not require %s", test.path, test.scheme)
		}
	}
}

func TestOpenAPIOAuthDiscoveryPreservesIssuerPrefix(t *testing.T) {
	spec := OpenAPISpecV1("https://analytics.example/tenant/")
	components := spec["components"].(map[string]any)
	scheme := components["securitySchemes"].(map[string]any)["oauth2"].(map[string]any)
	flow := scheme["flows"].(map[string]any)["authorizationCode"].(map[string]any)
	if flow["authorizationUrl"] != "https://analytics.example/tenant/oauth/authorize" || flow["tokenUrl"] != "https://analytics.example/tenant/oauth/token" {
		t.Fatalf("OAuth endpoint lost issuer prefix: %#v", flow)
	}
	paths := spec["paths"].(map[string]any)
	for _, path := range []string{"/.well-known/oauth-authorization-server/tenant", "/.well-known/oauth-protected-resource/tenant/api"} {
		servers := paths[path].(map[string]any)["servers"].([]map[string]string)
		if len(servers) != 1 || servers[0]["url"] != "https://analytics.example" {
			t.Errorf("discovery path %s must use issuer origin", path)
		}
	}
}

func TestOpenAPIOAuthIngestKeepsLegacyPayloadCompatible(t *testing.T) {
	paths := OpenAPISpecV1("https://analytics.example")["paths"].(map[string]any)
	for _, path := range []string{"/api/ingest/server/pageview", "/api/ingest/server/event"} {
		operation := paths[path].(map[string]any)["post"].(map[string]any)
		security := operation["security"].([]any)
		if len(security) != 3 {
			t.Fatalf("%s lost manual-client authentication: %#v", path, security)
		}
		body := operation["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
		if slices.Contains(body["required"].([]string), "site_id") {
			t.Errorf("%s requires OAuth-only field for legacy clients", path)
		}
		if _, ok := body["properties"].(map[string]any)["site_id"]; !ok {
			t.Errorf("%s omits OAuth site binding", path)
		}
	}
}

func TestAgentOpenAPIKeepsCodeClientRequirementsAndNoPreclaimCredentials(t *testing.T) {
	spec := OpenAPISpecV1("https://analytics.example/tenant")
	paths := spec["paths"].(map[string]any)
	token := paths["/oauth/token"].(map[string]any)["post"].(map[string]any)
	body := token["requestBody"].(map[string]any)["content"].(map[string]any)["application/x-www-form-urlencoded"].(map[string]any)["schema"].(map[string]any)
	alternatives := body["oneOf"].([]any)
	if len(alternatives) != 4 {
		t.Fatalf("missing grants: %#v", alternatives)
	}
	for _, value := range alternatives[:2] {
		required := value.(map[string]any)["required"].([]string)
		if !slices.Contains(required, "client_id") || !slices.Contains(required, "resource") {
			t.Fatal("ordinary code client lost binding")
		}
	}
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)
	registration := schemas["AgentRegistration"].(map[string]any)["properties"].(map[string]any)
	if registration["access_token"] != nil || registration["identity_assertion"] != nil {
		t.Fatal("preclaim credentials advertised")
	}
	tokens := schemas["AgentTokenResponse"].(map[string]any)["properties"].(map[string]any)
	if tokens["refresh_token"] != nil {
		t.Fatal("agent refresh token advertised")
	}
	runtime := OpenAPISpecV1ForRuntime("https://analytics.example/tenant", nil)["paths"].(map[string]any)
	for _, path := range []string{"/agent/identity", "/agent/identity/claim", "/oauth/agent-claim"} {
		if runtime[path] != nil {
			t.Fatalf("disabled agent route advertised: %s", path)
		}
	}
}
