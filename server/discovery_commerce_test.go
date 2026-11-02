package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hitkeep/config"
	json "hitkeep/jsonapi"
	"hitkeep/server/cloud"
)

func TestCommerceDiscoveryProfiles(t *testing.T) {
	for _, protocol := range []string{"acp", "ucp"} {
		t.Run(protocol, func(t *testing.T) {
			s := newDiscoveryServer()
			s.conf.CloudHosted = true
			s.conf.StripeSecretKey = "fixture-secret"
			s.conf.StripePriceProMonthly = "fixture-price"
			if !cloud.CommerceAvailable(s.conf) {
				t.Skip("requires billing build")
			}
			route := "/.well-known/ucp"
			endpoint := "https://analytics.example.test/tenant/api/cloud/commerce/ucp"
			if protocol == "acp" {
				route = "/.well-known/acp.json"
				endpoint = "https://analytics.example.test/tenant/api/cloud/commerce/acp"
			}
			get := discoveryRequest(s, http.MethodGet, route)
			if get.Code != http.StatusOK {
				t.Fatalf("profile=%d %s", get.Code, get.Body.String())
			}
			var profile map[string]any
			if err := json.Unmarshal(get.Body.Bytes(), &profile); err != nil {
				t.Fatal(err)
			}
			t.Logf("profile: %s", get.Body.String())
			if protocol == "acp" {
				meta := profile["protocol"].(map[string]any)
				versions := meta["supported_versions"].([]any)
				transports := profile["transports"].([]any)
				services := profile["capabilities"].(map[string]any)["services"].([]any)
				if meta["name"] != "acp" || meta["version"] != "2026-04-17" || len(versions) != 1 || versions[0] != "2026-04-17" || profile["api_base_url"] != endpoint || len(services) != 1 || services[0] != "checkout" || len(transports) != 1 || transports[0] != "rest" {
					t.Fatalf("invalid ACP discovery: %v", profile)
				}
			} else {
				meta := profile["ucp"].(map[string]any)
				service := meta["services"].(map[string]any)["dev.ucp.shopping"].([]any)[0].(map[string]any)
				capability := meta["capabilities"].(map[string]any)["dev.ucp.shopping.checkout"].([]any)[0].(map[string]any)
				if meta["version"] != "2026-08-25" || service["version"] != "2026-08-25" || service["endpoint"] != endpoint || service["transport"] != "rest" || service["schema"] != "https://ucp.dev/2026-08-25/services/shopping/rest.openapi.json" || capability["version"] != "2026-08-25" || capability["schema"] != "https://ucp.dev/2026-08-25/schemas/shopping/checkout.json" || len(meta["payment_handlers"].(map[string]any)) != 0 {
					t.Fatalf("invalid UCP discovery: %v", profile)
				}
			}
			for _, secret := range []string{"fixture-secret", "fixture-price", "malicious.example", "delegate_payment", "x402", "mpp"} {
				if strings.Contains(get.Body.String(), secret) {
					t.Fatalf("profile exposes %q", secret)
				}
			}
			if get.Header().Get("Access-Control-Allow-Origin") != "*" || get.Header().Get("ETag") == "" {
				t.Fatal("profile lacks public discovery metadata")
			}
			if head := discoveryRequest(s, http.MethodHead, route); head.Code != 200 || head.Body.Len() != 0 || head.Header().Get("ETag") != get.Header().Get("ETag") {
				t.Fatal("HEAD differs from GET")
			}
			if options := discoveryRequest(s, http.MethodOptions, route); options.Code != 204 {
				t.Fatalf("OPTIONS=%d", options.Code)
			}
			mux := http.NewServeMux()
			s.registerDiscoveryRoutes(mux)
			req := httptest.NewRequest("GET", route, nil)
			req.Header.Set("If-None-Match", get.Header().Get("ETag"))
			rec := httptest.NewRecorder()
			// Well-known discovery stays at the origin root for a prefixed deployment.
			s.publicBasePath = "/tenant/"
			s.stripPublicBasePath(mux).ServeHTTP(rec, req)
			if rec.Code != http.StatusNotModified {
				t.Fatalf("conditional root profile=%d", rec.Code)
			}
		})
	}
}

func TestCommerceDiscoveryUnavailable(t *testing.T) {
	for _, gate := range []string{"ai", "cloud", "key", "price", "store", "follower", "public-url"} {
		t.Run(gate, func(t *testing.T) {
			s := newDiscoveryServer()
			s.conf = &config.Config{PublicURL: "https://analytics.example", AIEnabled: true, CloudHosted: true, MCPPath: "/mcp", StripeSecretKey: "fixture-secret", StripePriceProMonthly: "fixture-price"}
			s.ctx.Config = s.conf
			switch gate {
			case "ai":
				s.conf.AIEnabled = false
			case "cloud":
				s.conf.CloudHosted = false
			case "key":
				s.conf.StripeSecretKey = ""
			case "price":
				s.conf.StripePriceProMonthly = ""
			case "store":
				s.ctx.Store = nil
			case "follower":
				s.ctx.Cluster = discoveryCluster(false)
			case "public-url":
				s.conf.PublicURL = "https://user:private@analytics.example"
			}
			for _, route := range []string{"/.well-known/acp.json", "/.well-known/ucp"} {
				if rec := discoveryRequest(s, "GET", route); rec.Code != http.StatusNotFound {
					t.Fatalf("%s gate: %s=%d", gate, route, rec.Code)
				}
			}
		})
	}
}
