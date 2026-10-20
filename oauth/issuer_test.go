package oauth

import "testing"

func TestIssuerResourcesAndDiscovery(t *testing.T) {
	for _, tc := range []struct{ base, mount, issuer, mcp, discovery string }{
		{"https://EXAMPLE.test/", "/mcp", "https://example.test", "https://example.test/mcp", "/.well-known/oauth-authorization-server"},
		{"https://example.test/hitkeep/", "/mcp", "https://example.test/hitkeep", "https://example.test/hitkeep/mcp", "/.well-known/oauth-authorization-server/hitkeep"},
		{"https://example.test/hitkeep", "/tools/analytics", "https://example.test/hitkeep", "https://example.test/hitkeep/tools/analytics", "/.well-known/oauth-authorization-server/hitkeep"},
	} {
		t.Run(tc.base+tc.mount, func(t *testing.T) {
			got, err := parseIssuer(tc.base, tc.mount)
			if err != nil {
				t.Fatal(err)
			}
			if got.Issuer != tc.issuer || got.APIResource != tc.issuer+"/api" || got.MCPResource != tc.mcp || got.AuthorizationMetadataPath != tc.discovery {
				t.Fatalf("incorrect canonical endpoints: %+v", got)
			}
			if got.APIResourceMetadataPath != "/.well-known/oauth-protected-resource"+got.Prefix+"/api" || got.MCPResourceMetadataPath != "/.well-known/oauth-protected-resource"+got.Prefix+tc.mount {
				t.Fatalf("incorrect resource metadata: %+v", got)
			}
		})
	}
}

func TestIssuerRejectsAmbiguousOrInsecureURLs(t *testing.T) {
	for _, raw := range []string{"http://example.test", "//example.test", "https://user:pass@example.test", "https://example.test/?q=1", "https://example.test/?", "https://example.test/#fragment", "https://example.test/a/../b", "https://example.test/a//b", "https://example.test/%2fadmin", "https://example.test:0", "https://example.test:65536", "https://example.test/\\foo", "https://example.test/\nfoo"} {
		t.Run(raw, func(t *testing.T) {
			if got, err := parseIssuer(raw, "/mcp"); err == nil {
				t.Fatalf("accepted unsafe issuer: %+v", got)
			}
		})
	}
	for _, mount := range []string{"", "mcp", "//other.test/mcp", "/api", "/mcp?x=1", "/mcp#x", "/mcp/../api", "/mcp/", "/%6dcp"} {
		t.Run(mount, func(t *testing.T) {
			if _, err := parseIssuer("https://example.test/hitkeep", mount); err == nil {
				t.Fatalf("accepted unsafe or overlapping MCP mount %q", mount)
			}
		})
	}
}

func TestIssuerRejectsEmptyFragmentDoubleRootAndEscapedWhitespace(t *testing.T) {
	for _, raw := range []string{"https://example.test/#", "https://example.test//", "https://example.test/a%20b", "https://example.test/a%0Ab"} {
		if _, err := parseIssuer(raw, "/mcp"); err == nil {
			t.Errorf("accepted ambiguous issuer %q", raw)
		}
	}
}
