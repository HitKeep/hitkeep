package system

import (
	"hitkeep/config"
	"hitkeep/server/shared"
	"testing"
)

func TestRuntimeOpenAPIOptionalAvailability(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		ctx := &shared.Context{Config: &config.Config{AIEnabled: enabled}}
		paths := OpenAPISpecV1ForRuntime("https://example.test", ctx)["paths"].(map[string]any)
		if (paths["/.well-known/agent-skills/index.json"] != nil) != enabled {
			t.Fatalf("AI=%v index availability", enabled)
		}
		for _, route := range []string{"/api/a2a/message:send", "/.well-known/agent-card.json", "/api/cloud/commerce/acp/checkout_sessions"} {
			if paths[route] != nil {
				t.Fatalf("unavailable handler advertised: %s", route)
			}
		}
		if paths["/api/status"] == nil || paths["/oauth/token"] == nil {
			t.Fatal("existing REST/OAuth paths lost")
		}
	}
	if OpenAPISpecV1("https://example.test")["paths"].(map[string]any)["/api/a2a/message:send"] == nil {
		t.Fatal("static reference lost optional API")
	}
	if OpenAPISpecV1ForRuntime("https://example.test", nil)["paths"] == nil {
		t.Fatal("nil runtime context lost ordinary documentation")
	}
}
