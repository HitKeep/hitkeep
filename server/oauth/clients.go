package oauth

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"uuid"

	"hitkeep/auth"
	"hitkeep/database"
	provider "hitkeep/oauth"
	"hitkeep/server/shared"
)

func (h *handler) registerClients(mux *http.ServeMux) {
	config := shared.HandlerConfig{HumanOnly: true, TeamCap: auth.CapTeamManageAPIClients, RateLimiter: h.ctx.ApiLimiter}
	mux.HandleFunc("GET /api/user/teams/{id}/oauth-clients", h.ctx.Handler(config, h.clients))
	mux.HandleFunc("POST /api/user/teams/{id}/oauth-clients", h.ctx.Handler(config, h.saveClient))
	mux.HandleFunc("PUT /api/user/teams/{id}/oauth-clients/{clientId}", h.ctx.Handler(config, h.saveClient))
	mux.HandleFunc("DELETE /api/user/teams/{id}/oauth-clients/{clientId}", h.ctx.Handler(config, h.deleteClient))
}

func (h *handler) clients(w http.ResponseWriter, r *http.Request) {
	teamID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		clientError(w, provider.ErrInvalidConsent)
		return
	}
	clients, err := h.ctx.OAuthProvider.ListClients(r.Context(), teamID)
	if err != nil {
		clientError(w, err)
		return
	}
	response(w, http.StatusOK, clients)
}

func (h *handler) saveClient(w http.ResponseWriter, r *http.Request) {
	teamID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		clientError(w, provider.ErrInvalidConsent)
		return
	}
	var request struct {
		Name         string   `json:"client_name"`
		URI          string   `json:"client_uri,omitempty"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.UnmarshalRead(http.MaxBytesReader(w, r.Body, 64<<10), &request, json.RejectUnknownMembers(true)); err != nil {
		clientError(w, provider.ErrInvalidConsent)
		return
	}
	create := r.Method == http.MethodPost
	c := provider.ClientRegistration{ID: r.PathValue("clientId"), Name: request.Name, URI: request.URI, RedirectURIs: request.RedirectURIs}
	saved, err := h.ctx.OAuthProvider.SaveClient(r.Context(), uuid.UUID(shared.GetUserIDFromContext(r)), teamID, c, create)
	if err != nil {
		clientError(w, err)
		return
	}
	status := http.StatusOK
	if create {
		status = http.StatusCreated
	}
	response(w, status, saved)
}

func (h *handler) deleteClient(w http.ResponseWriter, r *http.Request) {
	teamID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		clientError(w, provider.ErrInvalidConsent)
		return
	}
	if err := h.ctx.OAuthProvider.RevokeClient(r.Context(), uuid.UUID(shared.GetUserIDFromContext(r)), teamID, r.PathValue("clientId")); err != nil {
		clientError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func clientError(w http.ResponseWriter, err error) {
	status, code := http.StatusServiceUnavailable, "unavailable"
	switch {
	case errors.Is(err, provider.ErrInvalidConsent):
		status, code = http.StatusBadRequest, "invalid_request"
	case errors.Is(err, database.ErrOAuthNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, database.ErrOAuthCapacity):
		status, code = http.StatusConflict, "capacity"
	case errors.Is(err, database.ErrOAuthConflict):
		status, code = http.StatusConflict, "conflict"
	}
	response(w, status, map[string]string{"error": code})
}
