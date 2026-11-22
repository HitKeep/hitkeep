package mcpserver

import (
	"context"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hitkeep/analyticstools"
	"hitkeep/api"
	json "hitkeep/jsonapi"
)

func TestMCPPublishedSurfaceAudit(t *testing.T) {
	store, _, token := setupMCPStore(t)
	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, nil, nil, testMCPLogger())
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()
	init := session.InitializeResult()
	if init == nil || init.ProtocolVersion != "2026-07-28" {
		t.Fatalf("expected sessionless 2026-07-28 negotiation, got %+v", init)
	}
	if init.Capabilities == nil || init.Capabilities.Tools == nil || init.Capabilities.Resources == nil {
		t.Fatalf("unexpected advertised capabilities: %+v", init.Capabilities)
	}
	var capabilityFields map[string]json.RawMessage
	capabilityJSON, err := json.Marshal(init.Capabilities)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(capabilityJSON, &capabilityFields); err != nil {
		t.Fatal(err)
	}
	if _, advertised := capabilityFields["logging"]; advertised {
		t.Fatalf("deprecated logging capability advertised: %s", capabilityJSON)
	}
	if init.Capabilities.Tools.ListChanged || init.Capabilities.Resources.ListChanged || init.Capabilities.Resources.Subscribe {
		t.Fatalf("stateful capabilities advertised: %+v", init.Capabilities)
	}
	if init.Capabilities.Prompts == nil || init.Capabilities.Prompts.ListChanged {
		t.Fatalf("expected stateless prompts capability: %+v", init.Capabilities.Prompts)
	}
	if !strings.Contains(init.Instructions, "hitkeep_list_sites") || !strings.Contains(init.Instructions, "read-only") {
		t.Fatalf("instructions must route to site discovery and state the read-only boundary: %q", init.Instructions)
	}

	prompts, err := session.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	wantPrompts := []string{"hitkeep-ai-visibility-analyst", "hitkeep-analytics", "hitkeep-ecommerce-analyst", "hitkeep-tracking-verifier", "hitkeep-traffic-diagnosis"}
	gotPrompts := make([]string, 0, len(prompts.Prompts))
	for _, prompt := range prompts.Prompts {
		gotPrompts = append(gotPrompts, prompt.Name)
	}
	slices.Sort(gotPrompts)
	if !slices.Equal(gotPrompts, wantPrompts) {
		t.Fatalf("prompts = %v, want the public procedures %v", gotPrompts, wantPrompts)
	}
	prompt, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "hitkeep-traffic-diagnosis", Arguments: map[string]string{"question": "Why did traffic drop?"}})
	if err != nil || len(prompt.Messages) != 1 {
		t.Fatalf("GetPrompt: %v %+v", err, prompt)
	}
	if text, ok := prompt.Messages[0].Content.(*mcp.TextContent); !ok || !strings.Contains(text.Text, "Why did traffic drop?") {
		t.Fatalf("prompt did not carry the question: %+v", prompt.Messages[0].Content)
	}

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	wantTools := map[string]bool{
		"hitkeep_list_sites":                true,
		"hitkeep_get_site_overview":         true,
		"hitkeep_get_event_names":           true,
		"hitkeep_get_event_breakdown":       true,
		"hitkeep_get_ecommerce":             true,
		"hitkeep_get_web_vitals":            true,
		"hitkeep_get_ai_visibility":         true,
		"hitkeep_get_annotations":           true,
		"hitkeep_get_breakdown":             true,
		"hitkeep_get_opportunities":         true,
		"hitkeep_get_funnel_stats":          true,
		"hitkeep_get_qr_campaigns":          true,
		"hitkeep_get_search_console_status": true,
		"hitkeep_get_search_console":        true,
		"hitkeep_search_docs":               true,
		"hitkeep_get_doc":                   true,
		"hitkeep_get_api_reference":         true,
		"hitkeep_get_mcp_help":              true,
	}
	if len(tools.Tools) != len(wantTools) {
		t.Fatalf("expected %d tools, got %d: %v", len(wantTools), len(tools.Tools), toolNames(tools.Tools))
	}
	if tools.TTLMs != int((5*time.Minute)/time.Millisecond) || tools.CacheScope != "private" {
		t.Fatalf("tools/list cache = ttl %d scope %q, want 300000/private", tools.TTLMs, tools.CacheScope)
	}
	for _, tool := range tools.Tools {
		if !wantTools[tool.Name] {
			t.Fatalf("unexpected MCP tool %q", tool.Name)
		}
		if strings.TrimSpace(tool.Title) == "" || strings.TrimSpace(tool.Description) == "" {
			t.Fatalf("tool %q must have title and description", tool.Name)
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("tool %q must be marked read-only", tool.Name)
		}
		if !tool.Annotations.IdempotentHint {
			t.Fatalf("tool %q must be marked idempotent", tool.Name)
		}
		openWorld := tool.Annotations.OpenWorldHint != nil && *tool.Annotations.OpenWorldHint
		if strings.Contains(tool.Name, "_doc") || strings.Contains(tool.Name, "_api_reference") || strings.Contains(tool.Name, "search_docs") {
			if !openWorld {
				t.Fatalf("docs tool %q should declare open-world docs fetching", tool.Name)
			}
		} else if openWorld {
			t.Fatalf("analytics/local tool %q should not declare open-world behavior", tool.Name)
		}
		if isForbiddenToolName(tool.Name) {
			t.Fatalf("tool %q violates read-only aggregate-only surface policy", tool.Name)
		}
	}

	eventTool := findTool(tools.Tools, analyticstools.ToolEventBreakdown)
	var mcpInput, goaiInput struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type json.RawMessage `json:"type"`
		} `json:"properties"`
	}
	input, err := json.Marshal(eventTool.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(input, &mcpInput); err != nil {
		t.Fatal(err)
	}
	goaiTools := analyticstools.GoAI(analyticstools.Scope{}, analyticstools.EventBreakdown)
	if err := json.Unmarshal(goaiTools[0].InputSchema, &goaiInput); err != nil {
		t.Fatal(err)
	}
	for _, schema := range []struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type json.RawMessage `json:"type"`
		} `json:"properties"`
	}{mcpInput, goaiInput} {
		propertyType := string(schema.Properties["property_key"].Type)
		// MCP keeps nullable optional types; GoAI removes null for provider portability.
		stringType := propertyType == `"string"` || propertyType == `["string","null"]` || propertyType == `["null","string"]`
		if slices.Contains(schema.Required, "property_key") || !slices.Contains(schema.Required, "event_name") || !stringType {
			t.Fatalf("event inspection schema = %+v", schema)
		}
	}
	if string(goaiInput.Properties["property_key"].Type) != `"string"` {
		t.Fatal("GoAI optional key lost provider-portable string type")
	}
	var output struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	encoded, err := json.Marshal(eventTool.OutputSchema)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(output.Required, "property_keys") || slices.Contains(output.Required, "property_keys_truncated") {
		t.Fatal("discovery-only fields must stay optional")
	}
	for _, field := range []string{"breakdown", "property_keys", "property_keys_truncated"} {
		if len(output.Properties[field]) == 0 {
			t.Fatalf("event output missing %s", field)
		}
	}

	resources, err := session.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	for _, uri := range []string{helpMCPURI, helpMetricsURI, docsLLMSURI} {
		if !hasResource(resources.Resources, uri) {
			t.Fatalf("expected resource %s", uri)
		}
	}
	if resources.TTLMs != int((5*time.Minute)/time.Millisecond) || resources.CacheScope != "private" {
		t.Fatalf("resources/list cache = ttl %d scope %q, want 300000/private", resources.TTLMs, resources.CacheScope)
	}

	templates, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	if len(templates.ResourceTemplates) != 1 || templates.ResourceTemplates[0].URITemplate != "hitkeep://docs/{+path}" {
		t.Fatalf("unexpected resource templates: %+v", templates.ResourceTemplates)
	}
	if templates.TTLMs != int((5*time.Minute)/time.Millisecond) || templates.CacheScope != "private" {
		t.Fatalf("resources/templates cache = ttl %d scope %q, want 300000/private", templates.TTLMs, templates.CacheScope)
	}

	help, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: helpMCPURI})
	if err != nil {
		t.Fatalf("ReadResource help: %v", err)
	}
	if help.TTLMs != int(time.Hour/time.Millisecond) || help.CacheScope != "private" {
		t.Fatalf("help resource cache = ttl %d scope %q, want 3600000/private", help.TTLMs, help.CacheScope)
	}
}

func TestMCPFunnelStatsAudit(t *testing.T) {
	store, site, token := setupMCPStore(t)
	ctx := t.Context()
	funnel := &api.Funnel{SiteID: site.ID, Name: "Checkout", Steps: []api.FunnelStep{
		{Type: "path", Value: "/"},
		{Type: "path", Value: "/checkout"},
	}}
	if err := store.CreateFunnel(ctx, funnel); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	sessionID := uuid.New()
	for i, path := range []string{"/", "/checkout"} {
		if err := store.CreateHit(ctx, &api.Hit{
			SiteID: site.ID, SessionID: sessionID, PageID: uuid.New(), Path: path,
			Timestamp: now.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	conf := testMCPConfig(t, "")
	server := httptest.NewServer(NewHandler(conf, store, nil, nil, testMCPLogger()))
	defer server.Close()
	session := connectMCPClient(t, server.URL+conf.MCPPath, token)
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	tool := findTool(tools.Tools, analyticstools.ToolFunnelStats)
	if tool == nil {
		t.Fatal("missing funnel stats tool")
	}
	var schema jsonschema.Schema
	rawSchema, err := json.Marshal(tool.OutputSchema)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rawSchema, &schema); err != nil {
		t.Fatal(err)
	}
	statsSchema := schema.Properties["stats"]
	if statsSchema == nil || statsSchema.Properties["funnel_id"] == nil {
		t.Fatal("output schema missing stats.funnel_id")
	}
	if idType := statsSchema.Properties["funnel_id"].Type; idType != "string" {
		t.Errorf("tools/list stats.funnel_id type = %q, want string", idType)
	} else {
		t.Log("tools/list advertises stats.funnel_id as a string")
	}

	for _, tc := range []struct {
		name                          string
		from, to                      time.Time
		entries, completions, dropoff int
		rate                          float64
	}{
		{"matched traffic", now.Add(-time.Hour), now.Add(time.Hour), 2, 1, 1, 50},
		{"zero traffic", now.Add(-3 * time.Hour), now.Add(-2 * time.Hour), 0, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			from, to := tc.from.Format(time.RFC3339), tc.to.Format(time.RFC3339)
			res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      tool.Name,
				Arguments: map[string]any{"site_id": site.ID.String(), "funnel_id": funnel.ID.String(), "from": from, "to": to},
			})
			if err != nil {
				t.Fatalf("tools/call: %v", err)
			}
			if res.IsError {
				t.Fatalf("tools/call: %s", mcpToolErrorText(res))
			}
			var got struct {
				SiteID string           `json:"site_id"`
				From   string           `json:"from"`
				To     string           `json:"to"`
				Stats  *api.FunnelStats `json:"stats"`
			}
			if err := json.UnmarshalStrict([]byte(marshalMCPStructuredContent(t, res)), &got); err != nil {
				t.Fatal(err)
			}
			if got.SiteID != site.ID.String() || got.From != from || got.To != to {
				t.Fatalf("unexpected site or range: %+v", got)
			}
			want := &api.FunnelStats{
				FunnelID: funnel.ID, Name: funnel.Name, TotalEntries: tc.entries, TotalCompletions: tc.completions, OverallConversionRate: tc.rate,
				Steps: []api.FunnelStepStats{
					{StepIndex: 0, Name: "path: /", Visitors: tc.entries, ConversionRate: 100},
					{StepIndex: 1, Name: "path: /checkout", Visitors: tc.completions, Dropoff: tc.dropoff, ConversionRate: tc.rate},
				},
			}
			if diff := cmp.Diff(want, got.Stats); diff != "" {
				t.Fatalf("funnel stats (-want +got):\n%s", diff)
			}
			t.Logf("tools/call returned %d entries, %d completions, %.0f%% conversion", got.Stats.TotalEntries, got.Stats.TotalCompletions, got.Stats.OverallConversionRate)
		})
	}
}

func TestMCPDocsDisabledSurfaceAudit(t *testing.T) {
	store, _, token := setupMCPStore(t)
	conf := testMCPConfig(t, "")
	conf.MCPDocsEnabled = false
	handler := NewHandler(conf, store, nil, nil, testMCPLogger())
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range tools.Tools {
		if strings.Contains(tool.Name, "_doc") || strings.Contains(tool.Name, "_api_reference") || strings.Contains(tool.Name, "search_docs") {
			t.Fatalf("docs-disabled server should not expose docs tool %q", tool.Name)
		}
	}

	resources, err := session.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if hasResource(resources.Resources, docsLLMSURI) {
		t.Fatalf("docs-disabled server should not expose docs llms resource")
	}

	templates, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	if len(templates.ResourceTemplates) != 0 {
		t.Fatalf("docs-disabled server should not expose docs templates: %+v", templates.ResourceTemplates)
	}
}

func isForbiddenToolName(name string) bool {
	for _, part := range []string{
		"create",
		"delete",
		"update",
		"mutate",
		"write",
		"export_hits",
		"raw_hits",
		"billing",
		"takeout",
		"exclusion",
		"goal_mutation",
	} {
		if strings.Contains(name, part) {
			return true
		}
	}
	return false
}

func toolNames(tools []*mcp.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}
