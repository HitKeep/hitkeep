package system

import (
	"reflect"
	"strings"
	"testing"
)

func TestOpenAPISpecV1DocumentsHumanApprovedCloudHandoff(t *testing.T) {
	paths := requireMap(t, openAPISpecV1("https://analytics.example/prefix/"), "paths")
	operation := requireMap(t, requireMap(t, paths, "/api/cloud/plans"), "get")
	if description, _ := operation["description"].(string); !strings.Contains(description, "human approval") {
		t.Fatalf("catalog must describe human approval: %q", description)
	}
	responses := requireMap(t, operation, "responses")
	content := requireMap(t, requireMap(t, responses, "200"), "content")
	schema := requireMap(t, requireMap(t, content, "application/json"), "schema")
	prices := requireMap(t, requireMap(t, requireMap(t, schema, "items"), "properties"), "prices")
	offers := requireMap(t, prices, "properties")
	for _, interval := range []string{"monthly", "annual"} {
		t.Run(interval, func(t *testing.T) {
			offer := requireMap(t, offers, interval)
			if got := asStringSlice(t, offer["required"]); !reflect.DeepEqual(got, []string{"amount_minor", "currency"}) {
				t.Fatalf("required price fields changed: %v", got)
			}
			review := requireMap(t, requireMap(t, offer, "properties"), "purchase_review_url")
			if review["type"] != "string" || review["format"] != "uri" {
				t.Fatalf("review link must be an optional URI: %v", review)
			}
			if description, _ := review["description"].(string); !strings.Contains(description, "does not create a checkout session or complete a purchase") {
				t.Fatalf("review must not imply automatic payment: %q", description)
			}
		})
	}
}
