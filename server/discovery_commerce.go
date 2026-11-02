package server

import (
	"net/url"

	"hitkeep/appurl"
	"hitkeep/server/cloud"
)

func (s *Server) commerceAvailable() bool {
	return s.agentAvailable() && cloud.CommerceAvailable(s.conf) && s.ctx.Store != nil && (s.ctx.Cluster == nil || s.ctx.Cluster.IsLeader())
}

func (s *Server) acpProfile(base *url.URL) any {
	if !s.commerceAvailable() {
		return nil
	}
	return map[string]any{
		"protocol":     map[string]any{"name": "acp", "version": "2026-04-17", "supported_versions": []string{"2026-04-17"}, "documentation_url": appurl.Path(base.String(), "/openapi.json")},
		"api_base_url": appurl.Path(base.String(), "/api/cloud/commerce/acp"),
		"transports":   []string{"rest"},
		"capabilities": map[string]any{"services": []string{"checkout"}},
	}
}

func (s *Server) ucpProfile(base *url.URL) any {
	if !s.commerceAvailable() {
		return nil
	}
	return map[string]any{"ucp": map[string]any{
		"version": "2026-08-25",
		"services": map[string]any{"dev.ucp.shopping": []any{map[string]any{
			"version": "2026-08-25", "spec": "https://ucp.dev/2026-08-25/specification/overview/", "transport": "rest",
			"endpoint": appurl.Path(base.String(), "/api/cloud/commerce/ucp"), "schema": "https://ucp.dev/2026-08-25/services/shopping/rest.openapi.json",
		}}},
		"capabilities": map[string]any{"dev.ucp.shopping.checkout": []any{map[string]any{
			"version": "2026-08-25", "spec": "https://ucp.dev/2026-08-25/specification/shopping/checkout/", "schema": "https://ucp.dev/2026-08-25/schemas/shopping/checkout.json",
		}}},
		"payment_handlers": map[string]any{},
	}}
}
