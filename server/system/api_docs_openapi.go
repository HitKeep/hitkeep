package system

import (
	"maps"
	"strings"

	"hitkeep/server/cloud"
	"hitkeep/server/shared"
)

func OpenAPISpecV1(publicURL string) map[string]any {
	spec := openAPIDiscoverySpecV1(publicURL)
	addAgentRegistrationOpenAPI(spec)
	return spec
}

func openAPIDiscoverySpecV1(publicURL string) map[string]any {
	spec := openAPISpecV1(publicURL)
	paths := spec["paths"].(map[string]any)
	addOAuthActionSecurity(paths)
	maps.Copy(paths, agentDiscoveryPaths())
	maps.Copy(paths, commerceDiscoveryPaths())
	return spec
}

// OpenAPISpecV1ForRuntime advertises optional handlers only when their runtime prerequisites hold.
func OpenAPISpecV1ForRuntime(publicURL string, ctx *shared.Context) map[string]any {
	spec := openAPIDiscoverySpecV1(publicURL)
	if ctx != nil && ctx.Config != nil && ctx.Config.AIEnabled && ctx.OAuthProvider != nil && ctx.OAuthProvider.Ready() {
		addAgentRegistrationOpenAPI(spec)
	}
	paths := spec["paths"].(map[string]any)
	if ctx == nil || ctx.Config == nil || !ctx.Config.AIEnabled || ctx.OAuthProvider == nil || !ctx.OAuthProvider.Ready() {
		scopes := spec["components"].(map[string]any)["securitySchemes"].(map[string]any)["oauth2"].(map[string]any)["flows"].(map[string]any)["authorizationCode"].(map[string]any)["scopes"].(map[string]string)
		for _, scope := range []string{"goals:write", "annotations:write", "sites:write", "opportunities:write", "billing:write"} {
			delete(scopes, scope)
		}
		for path, item := range paths {
			for _, value := range item.(map[string]any) {
				operation, ok := value.(map[string]any)
				if !ok || (operation["x-hitkeep-oauth-scope"] == nil && !strings.HasPrefix(path, "/api/cloud/commerce/")) {
					continue
				}
				security := operation["security"].([]any)
				operation["security"] = security[:len(security)-1]
				delete(operation, "x-hitkeep-oauth-scope")
			}
		}
	}
	if ctx == nil || ctx.Config == nil || !ctx.Config.AIEnabled {
		for route := range agentDiscoveryPaths() {
			delete(paths, route)
		}
	}
	if ctx == nil || ctx.OAuthProvider == nil {
		delete(paths, "/.well-known/oauth-protected-resource")
	}
	if !ctx.A2AAvailable() {
		delete(paths, "/.well-known/agent-card.json")
		delete(paths, "/api/a2a/message:send")
	}
	commerce := ctx != nil && cloud.CommerceAvailable(ctx.Config) && ctx.Store != nil && (ctx.Cluster == nil || ctx.Cluster.IsLeader())
	if !commerce {
		for route := range paths {
			if strings.HasPrefix(route, "/api/cloud/commerce/") || route == "/.well-known/acp.json" || route == "/.well-known/ucp" {
				delete(paths, route)
			}
		}
	}
	return spec
}

func openAPISpecV1(publicURL string) map[string]any {
	components := openAPIV1Components()
	components["securitySchemes"].(map[string]any)["oauth2"] = oauthSecurityScheme(publicURL)
	paths := mergeOpenAPIPathMaps(openAPIV1Paths(), openAPIV1OAuthPaths(publicURL))
	addWordPressOpenAPI(components, paths)
	return map[string]any{
		"openapi":           "3.1.0",
		"jsonSchemaDialect": "https://spec.openapis.org/oas/3.1/dialect/base",
		"info": map[string]any{
			"title":       "HitKeep REST API",
			"version":     "v1",
			"description": "Complete HTTP API for HitKeep (session, API client, scoped OAuth, and public share-token endpoints). JSON request bodies must contain exactly one value, use valid UTF-8, and use unique, case-sensitive object member names; schema-closed operations also reject unknown members. While the shared database or an open tenant database is unavailable, database-dependent API and ingest routes return HTTP 503 using components.responses.DatabaseUnavailable.",
		},
		"servers": []map[string]string{{"url": publicURL}},
		"tags": []map[string]string{
			{"name": "System", "description": "Service health and API documentation endpoints."},
			{"name": "Ingest", "description": "Public tracking and event ingestion endpoints."},
			{"name": "Auth", "description": "Authentication, password, and sign-in flows."},
			{"name": "OAuth", "description": "Fosite authorization, consent, and connected applications."},
			{"name": "Integrations", "description": "Scoped site discovery and integration connections."},
			{"name": "Cloud", "description": "Managed Cloud endpoints, available only in hosted cloud builds with billing enabled. Operations marked x-internal are internal application interfaces; ACP/UCP commerce supports external assistants with explicit OAuth billing consent."},
			{"name": "User", "description": "Authenticated user profile, preferences, and security endpoints."},
			{"name": "Permissions", "description": "Authenticated permission context endpoints."},
			{"name": "Admin", "description": "Instance-level admin and membership management endpoints."},
			{"name": "Sites", "description": "Site lifecycle, stats, hits, and retention endpoints."},
			{"name": "Imports", "description": "Historical analytics import validation, upload, and lifecycle endpoints."},
			{"name": "Goals", "description": "Goal and goal-timeseries endpoints."},
			{"name": "Annotations", "description": "Team-visible notes on a site's timeline, shown on every chart."},
			{"name": "Funnels", "description": "Funnel CRUD and analytics endpoints."},
			{"name": "QR Campaigns", "description": "Dynamic QR campaign assets, print exports, redirects, attribution, and QR-scoped analytics."},
			{"name": "Share", "description": "Share-link management and public shared analytics endpoints."},
			{"name": "Takeout", "description": "Data export endpoints for user and site data."},
			{"name": "Reports", "description": "Scheduled email reports, delivery history, recipient confirmation, and unsubscribe controls."},
			{"name": "Teams", "description": "Tenant team membership and active-team context endpoints."},
			{"name": "Webhooks", "description": "Human-administered instance and site operational webhook endpoints."},
		},
		"components": components,
		"paths":      paths,
	}
}
