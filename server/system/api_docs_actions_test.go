package system

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"hitkeep/server/shared"
)

func TestOpenAPIDelegatedActionScopesMatchRegisteredRoutes(t *testing.T) {
	paths := OpenAPISpecV1("https://example.test")["paths"].(map[string]any)
	seen := make(map[string]bool)
	for _, source := range []string{"../sites/handlers.go", "../goals/handlers.go", "../annotations/handlers.go", "../opportunities/handlers.go"} {
		file, err := parser.ParseFile(token.NewFileSet(), source, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "HandleFunc" {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				return true
			}
			route, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(call.Args[1], func(node ast.Node) bool {
				field, ok := node.(*ast.KeyValueExpr)
				if !ok {
					return true
				}
				key, ok := field.Key.(*ast.Ident)
				if !ok || key.Name != "OAuthScope" {
					return true
				}
				value, ok := field.Value.(*ast.BasicLit)
				if !ok {
					t.Fatal("route scope must be explicit")
				}
				scope, err := strconv.Unquote(value.Value)
				if err != nil {
					t.Fatal(err)
				}
				method, path, _ := strings.Cut(route, " ")
				operation := paths[path].(map[string]any)[strings.ToLower(method)].(map[string]any)
				if operation["x-hitkeep-oauth-scope"] != scope {
					t.Fatalf("%s scope=%v want %s", route, operation["x-hitkeep-oauth-scope"], scope)
				}
				seen[route] = true
				return false
			})
			return false
		})
	}
	for path, item := range paths {
		for method, value := range item.(map[string]any) {
			operation, ok := value.(map[string]any)
			if ok && operation["x-hitkeep-oauth-scope"] != nil && !seen[strings.ToUpper(method)+" "+path] {
				t.Fatalf("unregistered delegated operation %s %s", method, path)
			}
		}
	}
	for _, route := range []string{"GET /api/sites/{id}/hits/export", "DELETE /api/sites/{id}", "POST /api/user/api-clients", "GET /api/user/oauth-grants"} {
		method, path, _ := strings.Cut(route, " ")
		if paths[path].(map[string]any)[strings.ToLower(method)].(map[string]any)["x-hitkeep-oauth-scope"] != nil {
			t.Fatalf("unsafe delegated operation %s", route)
		}
	}
	paths = OpenAPISpecV1ForRuntime("https://example.test", &shared.Context{})["paths"].(map[string]any)
	if paths["/api/sites/{id}/annotations"].(map[string]any)["post"].(map[string]any)["x-hitkeep-oauth-scope"] != nil {
		t.Fatal("unavailable OAuth actions advertised")
	}
}
