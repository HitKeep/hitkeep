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
