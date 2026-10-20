package server

import (
	"strings"
	"testing"

	"hitkeep/config"
	"hitkeep/database"
)

func TestOAuthOperatorClientRegistryValidation(t *testing.T) {
	for _, test := range []struct {
		name, registry string
		valid          bool
	}{
		{"default", "[]", true},
		{"existing zero config", "", true},
		{"explicit public client", `[{"client_id":"test-client","client_name":"Test client","redirect_uris":["https://client.example/callback"],"token_endpoint_auth_method":"none"}]`, true},
		{"null", "null", false},
		{"unknown client secret", `[{"client_id":"test-client","client_name":"Test client","redirect_uris":["https://client.example/callback"],"client_secret":"not-accepted"}]`, false},
		{"duplicate member", `[{"client_id":"one","client_id":"two"}]`, false},
		{"duplicate identity", `[{"client_id":"client","client_name":"One","redirect_uris":["https://client.example/callback"]},{"client_id":"client","client_name":"Two","redirect_uris":["https://other.example/callback"]}]`, false},
		{"insecure hosted redirect", `[{"client_id":"test-client","client_name":"Test client","redirect_uris":["http://client.example/callback"]}]`, false},
		{"bounded", "[" + strings.Repeat(" ", 64<<10) + "]", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			conf := &config.Config{PublicURL: "https://analytics.example", MCPPath: "/mcp", JWTSecret: strings.Repeat("k", 32), OAuthClients: test.registry}
			provider, err := newOAuthProvider(conf, &database.Store{}, func() bool { return true })
			if (err == nil) != test.valid || (provider != nil) != test.valid {
				t.Fatalf("registry validity = %t, want %t; error %v", err == nil, test.valid, err)
			}
		})
	}
}
