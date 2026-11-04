package system

import (
	"slices"
	"testing"
)

func TestWordPressContractsDescribeConstrainedEmbedAndRetryIdentity(t *testing.T) {
	spec := OpenAPISpecV1("https://analytics.example")
	paths := spec["paths"].(map[string]any)
	for _, path := range []string{"/api/integrations/sites/{id}/embed", "/api/embed/sites/{id}/site", "/api/oauth/sites"} {
		if paths[path] == nil {
			t.Errorf("missing WordPress contract %s", path)
		}
	}
	schema := serverEventIngestSchema()
	if schema["properties"].(map[string]any)["event_id"] == nil {
		t.Fatal("event retry identity absent")
	}
	for _, optional := range []string{"visitor_ip", "user_agent", "event_id"} {
		if slices.Contains(schema["required"].([]string), optional) {
			t.Fatalf("%s must be optional for anonymous durable events", optional)
		}
	}
}
