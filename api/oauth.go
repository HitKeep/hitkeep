package api

import (
	"time"
	"uuid"
)

type OAuthClientSummary struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	CallbackOrigin string `json:"callback_origin,omitempty"`
}
type OAuthSite struct {
	ID     uuid.UUID `json:"id"`
	Domain string    `json:"domain"`
	Name   string    `json:"name,omitempty"`
}
type OAuthTeam struct {
	ID    uuid.UUID   `json:"id"`
	Name  string      `json:"name"`
	Sites []OAuthSite `json:"sites"`
}
type OAuthTeamSummary struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}
type OAuthConsent struct {
	Client    OAuthClientSummary `json:"client"`
	Resource  string             `json:"resource"`
	Scopes    []string           `json:"scopes"`
	ExpiresAt time.Time          `json:"expires_at"`
	CSRFToken string             `json:"csrf_token"`
	Teams     []OAuthTeam        `json:"teams"`
}
type OAuthAuthorizedApp struct {
	ID         uuid.UUID          `json:"id"`
	Client     OAuthClientSummary `json:"client"`
	Team       OAuthTeamSummary   `json:"team"`
	Sites      []OAuthSite        `json:"sites"`
	Scopes     []string           `json:"scopes"`
	Resource   string             `json:"resource"`
	CreatedAt  time.Time          `json:"created_at"`
	ExpiresAt  time.Time          `json:"expires_at"`
	LastUsedAt *time.Time         `json:"last_used_at,omitempty"`
}
type IntegrationSite struct {
	ID     uuid.UUID `json:"id"`
	Domain string    `json:"domain"`
}
type IntegrationTrackingDomain struct {
	Hostname  string `json:"hostname"`
	ScriptURL string `json:"script_url"`
}
type IntegrationTrackingDomains struct {
	DefaultScriptURL    string                      `json:"default_script_url"`
	ActiveCustomDomains []IntegrationTrackingDomain `json:"active_custom_domains"`
}

type IntegrationPreview struct {
	SiteID         uuid.UUID `json:"site_id"`
	Start          time.Time `json:"start"`
	End            time.Time `json:"end"`
	TotalPageviews int       `json:"total_pageviews"`
	UniqueSessions int       `json:"unique_sessions"`
	BounceRate     float64   `json:"bounce_rate"`
}
