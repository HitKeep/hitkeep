package server

import (
	"context"
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"uuid"

	legacyuuid "github.com/google/uuid"

	"hitkeep/analyticstools"
	"hitkeep/appurl"
	"hitkeep/auth"
	wire "hitkeep/jsonapi"
	"hitkeep/oauth"
	"hitkeep/server/shared"
)

var errA2AAnalyticsUnavailable = errors.New("analytics unavailable")

func (s *Server) registerA2ARoutes(mux *http.ServeMux) {
	authenticated := func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		challenge := `Bearer resource_metadata="` + s.ctx.OAuthProvider.ResourceMetadataURL("api") + `"`
		if len(r.Header.Values("Authorization")) != 1 || len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || !oauth.IsOAuthToken(parts[1]) {
			w.Header().Set("WWW-Authenticate", challenge)
			a2aError(w, 401, "UNAUTHENTICATED", "API-resource OAuth token required", "")
			return
		}
		grant, err := s.ctx.OAuthProvider.Authenticate(r.Context(), parts[1], s.ctx.OAuthProvider.Resource("api"))
		if errors.Is(err, oauth.ErrUnavailable) {
			a2aError(w, 503, "UNAVAILABLE", "OAuth temporarily unavailable", "")
			return
		}
		if err != nil || grant == nil {
			w.Header().Set("WWW-Authenticate", challenge)
			a2aError(w, 401, "UNAUTHENTICATED", "Invalid OAuth token", "")
			return
		}
		if !slices.Contains(grant.Scopes, oauth.ScopeSitesRead) || !slices.Contains(grant.Scopes, oauth.ScopeAnalyticsRead) {
			w.Header().Set("WWW-Authenticate", `Bearer error="insufficient_scope", scope="sites:read analytics:read"`)
			a2aError(w, 403, "PERMISSION_DENIED", "Required scopes: sites:read analytics:read", "")
			return
		}
		if r.URL.Path != "/api/a2a/message:send" || r.Method != http.MethodPost {
			if strings.HasPrefix(r.URL.Path, "/api/a2a/tasks") {
				a2aError(w, 404, "NOT_FOUND", "This synchronous agent has no tasks", "TASK_NOT_FOUND")
			} else {
				a2aError(w, 400, "FAILED_PRECONDITION", "Only synchronous SendMessage is supported", "UNSUPPORTED_OPERATION")
			}
			return
		}
		serveA2AMessage(w, r, s.a2aScope(grant), analyticstools.Analytics())
	}
	handler := s.ctx.Handler(shared.HandlerConfig{RateLimiter: s.ctx.ApiLimiter}, func(w http.ResponseWriter, r *http.Request) {
		if !s.ctx.A2AAvailable() {
			http.NotFound(w, r)
			return
		}
		authenticated(w, r)
	})
	mux.HandleFunc("/api/a2a/", handler)
}

func (s *Server) a2aScope(grant *auth.OAuthGrant) analyticstools.Scope {
	return analyticstools.Scope{MaxRangeDays: 31, Resolve: func(ctx context.Context, siteID legacyuuid.UUID) (analyticstools.Site, error) {
		if !s.ctx.A2AAvailable() || grant == nil || !slices.Contains(grant.Scopes, oauth.ScopeSitesRead) {
			return analyticstools.Site{}, shared.ErrOAuthAccessDenied
		}
		if err := s.ctx.AuthorizeOAuthSite(ctx, grant, uuid.UUID(siteID), s.ctx.OAuthProvider.Resource("api"), oauth.ScopeAnalyticsRead, auth.PermSiteView); err != nil {
			return analyticstools.Site{}, shared.ErrOAuthAccessDenied
		}
		store, err := s.ctx.AnalyticsStore(ctx, siteID)
		if err != nil {
			return analyticstools.Site{}, errA2AAnalyticsUnavailable
		}
		return analyticstools.Site{ID: siteID, UserID: legacyuuid.UUID(grant.UserID), Control: s.ctx.Store, Analytics: store}, nil
	}}
}

type a2aPart struct {
	MediaType string                     `json:"mediaType"`
	Metadata  map[string]wire.RawMessage `json:"metadata"`
	Data      *struct {
		Tool      string          `json:"tool"`
		Arguments wire.RawMessage `json:"arguments"`
	} `json:"data"`
}

type a2aRequest struct {
	Tenant   string                     `json:"tenant"`
	Metadata map[string]wire.RawMessage `json:"metadata"`
	Message  struct {
		MessageID        string                     `json:"messageId"`
		Metadata         map[string]wire.RawMessage `json:"metadata"`
		Extensions       []string                   `json:"extensions"`
		ReferenceTaskIDs []string                   `json:"referenceTaskIds"`
		ContextID        string                     `json:"contextId"`
		TaskID           string                     `json:"taskId"`
		Role             string                     `json:"role"`
		Parts            []a2aPart                  `json:"parts"`
	} `json:"message"`
	Configuration struct {
		AcceptedOutputModes    []string        `json:"acceptedOutputModes"`
		PushNotificationConfig wire.RawMessage `json:"taskPushNotificationConfig"`
		HistoryLength          int             `json:"historyLength"`
		ReturnImmediately      bool            `json:"returnImmediately"`
	} `json:"configuration"`
}

func serveA2AMessage(w http.ResponseWriter, r *http.Request, scope analyticstools.Scope, tools []analyticstools.Tool) {
	version := r.Header.Get("A2a-Version")
	if version == "" {
		version = r.URL.Query().Get("A2A-Version")
	}
	if version != "1.0" {
		a2aError(w, 400, "INVALID_ARGUMENT", "Supported A2A version: 1.0", "VERSION_NOT_SUPPORTED")
		return
	}
	var request a2aRequest
	if err := wire.UnmarshalRead(http.MaxBytesReader(w, r.Body, 64<<10), &request, json.RejectUnknownMembers(true)); err != nil {
		a2aError(w, 400, "INVALID_ARGUMENT", "Expected one structured analytics message", "")
		return
	}
	if request.Tenant != "" {
		a2aError(w, 400, "INVALID_ARGUMENT", "Tenant routing is not supported", "")
		return
	}
	if len(request.Configuration.PushNotificationConfig) > 0 && string(request.Configuration.PushNotificationConfig) != "null" {
		a2aError(w, 400, "INVALID_ARGUMENT", "Push notifications are unavailable", "PUSH_NOTIFICATION_NOT_SUPPORTED")
		return
	}
	message := request.Message
	if len(message.ReferenceTaskIDs) > 0 {
		a2aError(w, 404, "NOT_FOUND", "This synchronous agent has no tasks", "TASK_NOT_FOUND")
		return
	}
	if message.TaskID != "" {
		a2aError(w, 404, "NOT_FOUND", "This synchronous agent has no tasks", "TASK_NOT_FOUND")
		return
	}
	if len(request.Configuration.AcceptedOutputModes) > 0 && !slices.Contains(request.Configuration.AcceptedOutputModes, "application/json") {
		a2aError(w, 400, "INVALID_ARGUMENT", "Output mode must accept application/json", "CONTENT_TYPE_NOT_SUPPORTED")
		return
	}
	if message.MessageID == "" || len(message.MessageID) > 256 || len(message.ContextID) > 256 || message.Role != "ROLE_USER" || len(message.Parts) != 1 || message.Parts[0].Data == nil {
		a2aError(w, 400, "INVALID_ARGUMENT", "Send one data part with tool and arguments", "")
		return
	}
	data := message.Parts[0].Data
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	for _, definition := range tools {
		if definition.Name != data.Tool {
			continue
		}
		tool := analyticstools.GoAI(scope, definition)[0]
		output, err := tool.Execute(ctx, data.Arguments)
		if err != nil {
			if errors.Is(err, shared.ErrOAuthAccessDenied) {
				a2aError(w, 403, "PERMISSION_DENIED", "Site access denied", "")
				return
			}
			a2aError(w, 400, "INVALID_ARGUMENT", err.Error(), "")
			return
		}
		contextID := message.ContextID
		if contextID == "" {
			contextID = uuid.New().String()
		}
		a2aResponse(w, 200, map[string]any{"message": map[string]any{"messageId": uuid.New().String(), "contextId": contextID, "role": "ROLE_AGENT", "parts": []any{map[string]any{"data": wire.RawMessage(output), "mediaType": "application/json"}}}})
		return
	}
	a2aError(w, 400, "INVALID_ARGUMENT", "Unknown analytics tool", "")
}

func a2aResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/a2a+json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = wire.MarshalWrite(w, value)
}

func a2aError(w http.ResponseWriter, code int, status, message, reason string) {
	body := map[string]any{"code": code, "status": status, "message": message}
	if reason != "" {
		body["details"] = []any{map[string]string{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": reason, "domain": "a2a-protocol.org"}}
	}
	a2aResponse(w, code, map[string]any{"error": body})
}

func (s *Server) a2aCard(base *url.URL) any {
	if !s.ctx.A2AAvailable() {
		return nil
	}
	skills := []map[string]any{}
	tools := analyticstools.Analytics()
	adapted := analyticstools.GoAI(analyticstools.Scope{MaxRangeDays: 31}, tools...)
	for i, tool := range tools {
		skills = append(skills, map[string]any{"id": tool.Name, "name": tool.Title, "description": tool.Description + " Send one data part: {tool: skill id, arguments: tool input including site_id}. Maximum range 31 days. Input JSON Schema: " + string(adapted[i].InputSchema), "tags": []string{"analytics", "read-only"}})
	}
	return map[string]any{
		"name": "HitKeep Analytics", "description": "Synchronous structured read-only aggregate analytics for explicitly consented sites. No natural-language model, tasks, streaming, or push notifications.", "version": s.conf.Version,
		"supportedInterfaces": []any{map[string]string{"url": appurl.Path(base.String(), "/api/a2a"), "protocolBinding": "HTTP+JSON", "protocolVersion": "1.0"}},
		"capabilities":        map[string]bool{"streaming": false, "pushNotifications": false},
		"defaultInputModes":   []string{"application/json"}, "defaultOutputModes": []string{"application/json"}, "skills": skills,
		"securitySchemes":      map[string]any{"oauth": map[string]any{"oauth2SecurityScheme": map[string]any{"oauth2MetadataUrl": base.Scheme + "://" + base.Host + s.ctx.OAuthProvider.DiscoveryPaths()[0], "flows": map[string]any{"authorizationCode": map[string]any{"authorizationUrl": appurl.Path(base.String(), "/oauth/authorize"), "tokenUrl": appurl.Path(base.String(), "/oauth/token"), "pkceRequired": true, "scopes": map[string]string{"sites:read": "Consented sites", "analytics:read": "Aggregate analytics"}}}}}},
		"securityRequirements": []any{map[string]any{"schemes": map[string]any{"oauth": map[string]any{"list": []string{"sites:read", "analytics:read"}}}}},
		"documentationUrl":     appurl.Path(base.String(), "/auth.md"),
	}
}
