package system

import "strings"

// addOAuthActionSecurity keeps ordinary REST schemas and their existing authentication alternatives.
func addOAuthActionSecurity(paths map[string]any) {
	scopes := map[string]string{
		"GET /api/sites":                                      "sites:read",
		"GET /api/sites/{id}/stats":                           "analytics:read",
		"GET /api/sites/{id}/setup-state":                     "analytics:read",
		"GET /api/sites/{id}/tracking/status":                 "analytics:read",
		"GET /api/sites/{id}/tracking-domain-options":         "analytics:read",
		"GET /api/sites/{id}/ecommerce":                       "analytics:read",
		"GET /api/sites/{id}/ecommerce/timeseries":            "analytics:read",
		"GET /api/sites/{id}/ecommerce/products":              "analytics:read",
		"GET /api/sites/{id}/ecommerce/sources":               "analytics:read",
		"GET /api/sites/{id}/ai-activity":                     "analytics:read",
		"GET /api/sites/{id}/web-vitals/summary":              "analytics:read",
		"GET /api/sites/{id}/web-vitals/timeseries":           "analytics:read",
		"GET /api/sites/{id}/web-vitals/pages":                "analytics:read",
		"GET /api/sites/{id}/web-vitals/breakdown":            "analytics:read",
		"PUT /api/sites/{id}/domain":                          "sites:write",
		"PUT /api/sites/{id}/retention":                       "sites:write",
		"GET /api/sites/{id}/exclusions":                      "sites:write",
		"POST /api/sites/{id}/exclusions":                     "sites:write",
		"DELETE /api/sites/{id}/exclusions/{ruleID}":          "sites:write",
		"GET /api/sites/{id}/goals":                           "analytics:read",
		"GET /api/sites/{id}/goals/timeseries":                "analytics:read",
		"POST /api/sites/{id}/goals":                          "goals:write",
		"PUT /api/sites/{id}/goals/{goalID}":                  "goals:write",
		"DELETE /api/sites/{id}/goals/{goalID}":               "goals:write",
		"GET /api/sites/{id}/funnels":                         "analytics:read",
		"GET /api/sites/{id}/funnels/timeseries":              "analytics:read",
		"POST /api/sites/{id}/funnels":                        "goals:write",
		"PUT /api/sites/{id}/funnels/{funnelID}":              "goals:write",
		"DELETE /api/sites/{id}/funnels/{funnelID}":           "goals:write",
		"GET /api/sites/{id}/funnels/{funnelID}/stats":        "analytics:read",
		"GET /api/sites/{id}/annotations":                     "analytics:read",
		"POST /api/sites/{id}/annotations":                    "annotations:write",
		"PUT /api/sites/{id}/annotations/{annotationID}":      "annotations:write",
		"DELETE /api/sites/{id}/annotations/{annotationID}":   "annotations:write",
		"GET /api/sites/{id}/opportunities":                   "analytics:read",
		"GET /api/sites/{id}/opportunities/digest-preview":    "analytics:read",
		"POST /api/sites/{id}/opportunities/generate":         "opportunities:write",
		"PATCH /api/sites/{id}/opportunities/{opportunityID}": "opportunities:write",
	}
	for route, scope := range scopes {
		method, path, _ := strings.Cut(route, " ")
		operation := paths[path].(map[string]any)[strings.ToLower(method)].(map[string]any)
		operation["security"] = append(operation["security"].([]any), map[string]any{"oauth2": []string{scope}})
		operation["x-hitkeep-oauth-scope"] = scope
		operation["description"] = operation["description"].(string) + " Delegated OAuth requires AIEnabled, the API resource, this operation’s exact scope, explicit site consent, and current site/team authority. Existing session and API-client authorization remains unchanged."
	}
}
