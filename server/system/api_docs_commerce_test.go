package system

import (
	"reflect"
	"strings"
	"testing"

	"hitkeep/config"
	"hitkeep/database"
	"hitkeep/server/cloud"
	"hitkeep/server/shared"
)

func TestCommerceOpenAPIUsesDelegatedBillingAndHumanPayment(t *testing.T) {
	paths := openAPIV1CommercePaths()
	for path, raw := range paths {
		if strings.Contains(path, "mpp") || strings.Contains(path, "x402") {
			t.Fatalf("advertised unimplemented rail %s", path)
		}
		for method, raw := range raw.(map[string]any) {
			operation := raw.(map[string]any)
			if operation["x-internal"] == true {
				t.Fatalf("%s %s hides supported offsite commerce", method, path)
			}
			if !reflect.DeepEqual(operation["security"], append(secCookie(), map[string]any{"oauth2": []string{"billing:write"}})) {
				t.Fatalf("%s %s lacks delegated billing authorization", method, path)
			}
			description := operation["description"].(string)
			for _, text := range []string{"AI enabled", "continue_url", "complete and paid", "team.manage_settings", "API-resource OAuth bearer token", "billing:write", "consented team", "revocation", "does not collect payment credentials"} {
				if !strings.Contains(description, text) {
					t.Fatalf("missing %s in %s", text, path)
				}
			}
		}
	}
	if len(paths) != 8 {
		t.Fatalf("checkout paths=%d", len(paths))
	}
	if OpenAPISpecV1("https://analytics.example.com")["paths"].(map[string]any)["/api/cloud/billing/checkout"].(map[string]any)["post"].(map[string]any)["x-internal"] != true {
		t.Fatal("dashboard-only billing must remain internal")
	}
}

func TestRuntimeCommerceDoesNotAdvertiseUnconfiguredOAuth(t *testing.T) {
	conf := &config.Config{AIEnabled: true, CloudHosted: true, StripeSecretKey: "fixture-key", StripePriceProMonthly: "fixture-price"}
	if !cloud.CommerceAvailable(conf) {
		t.Skip("requires billing build")
	}
	spec := OpenAPISpecV1ForRuntime("https://analytics.example.com", &shared.Context{Config: conf, Store: &database.Store{}})
	operation := spec["paths"].(map[string]any)["/api/cloud/commerce/acp/checkout_sessions"].(map[string]any)["post"].(map[string]any)
	if !reflect.DeepEqual(operation["security"], secCookie()) {
		t.Fatal("unconfigured OAuth advertised alongside available browser Checkout")
	}
}

func TestRuntimeCommerceDiscoveryAvailability(t *testing.T) {
	for _, configured := range []bool{false, true} {
		conf := &config.Config{AIEnabled: true, CloudHosted: true}
		if configured {
			conf.StripeSecretKey = "fixture-key"
			conf.StripePriceProMonthly = "fixture-price"
		}
		spec := OpenAPISpecV1ForRuntime("https://analytics.example.com", &shared.Context{Config: conf, Store: &database.Store{}})
		for _, path := range []string{"/.well-known/acp.json", "/.well-known/ucp"} {
			item, ok := spec["paths"].(map[string]any)[path]
			if ok != cloud.CommerceAvailable(conf) {
				t.Fatalf("%s: configured=%t advertised=%t", path, configured, ok)
			}
			if ok && item.(map[string]any)["get"].(map[string]any)["security"] != nil {
				t.Fatalf("public discovery requires authentication: %s", path)
			}
		}
	}
}
