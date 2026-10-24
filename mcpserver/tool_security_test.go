package mcpserver

import (
	"context"
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hitkeep/oauth"
)

func TestMCPToolSecurityWireDescriptors(t *testing.T) {
	store, _, token := setupMCPStore(t)
	conf := testMCPConfig(t, "")
	conf.PublicURL = "https://analytics.example/prefix"
	provider, err := oauth.New(store, oauth.Options{
		Issuer: conf.PublicURL, MCPPath: conf.MCPPath, MCPEnabled: true,
		BrowserSigningKey: []byte(strings.Repeat("b", 32)), IsLeader: func() bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := oauth.New(store, oauth.Options{
		Issuer: conf.PublicURL, MCPPath: conf.MCPPath, MCPEnabled: false,
		BrowserSigningKey: []byte(strings.Repeat("b", 32)), IsLeader: func() bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, protocol := range []string{"2026-07-28", "2025-06-18"} {
		t.Run(protocol, func(t *testing.T) {
			baselineHandler := NewHandler(conf, store, nil, nil, testMCPLogger())
			if protocol == "2025-06-18" {
				descriptorWireResult(t, descriptorRequest(t, baselineHandler, token, protocol, "initialize", map[string]any{
					"protocolVersion": protocol, "capabilities": map[string]any{},
					"clientInfo": map[string]any{"name": "descriptor-test", "version": "test"},
				}))
			}
			baseline := descriptorWireResult(t, descriptorRequest(t, baselineHandler, token, protocol, "tools/list", nil))
			for _, tc := range []struct {
				name      string
				provider  *oauth.Provider
				protected bool
			}{
				{name: "OAuth configured", provider: provider, protected: true},
				{name: "legacy API client only"},
				{name: "MCP OAuth resource disabled", provider: disabled},
			} {
				t.Run(tc.name, func(t *testing.T) {
					handler := NewHandler(conf, store, nil, nil, testMCPLogger(), tc.provider)
					if protocol == "2025-06-18" {
						descriptorWireResult(t, descriptorRequest(t, handler, token, protocol, "initialize", map[string]any{
							"protocolVersion": protocol, "capabilities": map[string]any{},
							"clientInfo": map[string]any{"name": "descriptor-test", "version": "test"},
						}))
					} else {
						discovery := descriptorWireResult(t, descriptorRequest(t, handler, token, protocol, "server/discover", nil))
						if discovery["capabilities"] == nil {
							t.Fatal("discovery lost capabilities")
						}
					}
					result := descriptorWireResult(t, descriptorRequest(t, handler, token, protocol, "tools/list", nil))
					tools, ok := result["tools"].([]any)
					if !ok || len(tools) != 18 {
						t.Fatalf("published tool inventory changed: %T/%d", result["tools"], len(tools))
					}
					for _, value := range tools {
						tool := value.(map[string]any)
						schemes, exists := tool["securitySchemes"]
						if exists != tc.protected {
							t.Fatalf("%s securitySchemes present=%v, want %v", tool["name"], exists, tc.protected)
						}
						meta, _ := tool["_meta"].(map[string]any)
						if tc.protected {
							want := []any{map[string]any{"type": "oauth2", "scopes": []any{"sites:read", "analytics:read"}}}
							if !reflect.DeepEqual(schemes, want) || !reflect.DeepEqual(meta["securitySchemes"], want) {
								t.Fatalf("%s must declare and mirror the exact read scopes", tool["name"])
							}
							delete(tool, "securitySchemes")
							delete(meta, "securitySchemes")
							if len(meta) == 0 {
								delete(tool, "_meta")
							}
						} else if _, exists := meta["securitySchemes"]; exists {
							t.Fatalf("%s advertises unavailable OAuth in _meta", tool["name"])
						}
					}
					if !reflect.DeepEqual(result, baseline) {
						t.Fatal("descriptor policy changed schemas, annotations, order, cache hints or list fields")
					}

					for _, bearer := range []string{"", "invalid-token"} {
						response := descriptorRequest(t, handler, bearer, protocol, "tools/list", nil)
						if response.Code != http.StatusUnauthorized || response.Header().Get("WWW-Authenticate") == "" || strings.Contains(response.Body.String(), "securitySchemes") {
							t.Fatalf("invalid bearer reached MCP: status %d", response.Code)
						}
					}
					cookieOnly := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						r.Header.Del("Authorization")
						handler.ServeHTTP(w, r)
					})
					if response := descriptorRequest(t, cookieOnly, token, protocol, "tools/list", nil); response.Code != http.StatusUnauthorized {
						t.Fatal("a cookie replaced bearer authentication")
					}
					denied := descriptorWireResult(t, descriptorRequest(t, handler, token, protocol, "tools/call", map[string]any{
						"name": "hitkeep_get_site_overview", "arguments": map[string]any{"site_id": "00000000-0000-0000-0000-000000000001"},
					}))
					meta, _ := denied["_meta"].(map[string]any)
					if denied["isError"] != true || meta["mcp/www_authenticate"] != nil {
						t.Fatal("site denial must remain a tool error without a relinking challenge")
					}
				})
			}
		})
	}
}

func descriptorRequest(t *testing.T, handler http.Handler, token, protocol, method string, params map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	if params == nil {
		params = map[string]any{}
	}
	if protocol == "2026-07-28" {
		params["_meta"] = map[string]any{
			"io.modelcontextprotocol/protocolVersion":    protocol,
			"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "descriptor-test", "version": "test"},
			"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		}
	}
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "https://analytics.example/mcp", strings.NewReader(string(body))).WithContext(t.Context())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", protocol)
	req.Header.Set("Mcp-Method", method)
	if name, ok := params["name"].(string); ok {
		req.Header.Set("Mcp-Name", name)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	// A dashboard cookie cannot replace the bearer token, even on discovery.
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Header().Get("Mcp-Session-Id") != "" {
		t.Fatal("descriptor response introduced a session identifier")
	}
	return response
}

func descriptorWireResult(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("MCP HTTP status %d: %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Result map[string]any `json:"result"`
		Error  any            `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || envelope.Error != nil || envelope.Result == nil {
		t.Fatalf("invalid MCP result: error=%v protocol=%v", err, envelope.Error)
	}
	return envelope.Result
}

func TestMCPToolSecurityPreservesSourceAndPagination(t *testing.T) {
	store, _, _ := setupMCPStore(t)
	provider, err := oauth.New(store, oauth.Options{Issuer: "https://analytics.example", MCPPath: "/mcp", MCPEnabled: true, BrowserSigningKey: []byte(strings.Repeat("b", 32)), IsLeader: func() bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	svc := &service{oauthProvider: provider}
	tool := &mcp.Tool{Name: "example", Meta: mcp.Meta{"retained": "value"}}
	listed := &mcp.ListToolsResult{Tools: []*mcp.Tool{tool}}
	listed.NextCursor = "next-page"
	listed.TTLMs = int(time.Minute / time.Millisecond)
	listed.CacheScope = "private"
	listed.Meta = mcp.Meta{"retained": "result"}
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) { return listed, nil }
	result, err := svc.toolSecurityMiddleware()(next)(t.Context(), "tools/list", nil)
	if err != nil {
		t.Fatal(err)
	}
	protected, ok := result.(*oauthToolsResult)
	if !ok || protected.NextCursor != listed.NextCursor || protected.TTLMs != listed.TTLMs || protected.CacheScope != "private" || protected.Meta["retained"] != "result" {
		t.Fatal("descriptor adapter lost list metadata or pagination")
	}
	if protected.Tools[0].Meta["retained"] != "value" || protected.Tools[0].Tool == tool || tool.Meta["securitySchemes"] != nil {
		t.Fatal("descriptor adapter mutated the shared source tool or lost existing metadata")
	}
}

func TestMCPHelpDescribesScopedOAuthAndLegacyClients(t *testing.T) {
	help := mcpHelpMarkdown()
	for _, want := range []string{"Integration → Assistants", "canonical", "OAuth", "sites:read analytics:read", "selected sites", "Connected applications", "API client", "Dashboard cookies do not authenticate MCP", "comparison_basis", "property_key", "kpis"} {
		if !strings.Contains(help, want) {
			t.Errorf("MCP help omits %q", want)
		}
	}
}
