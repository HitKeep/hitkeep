package oauth

import (
	"bytes"
	"crypto/subtle"
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"uuid"

	googleuuid "github.com/google/uuid"

	"hitkeep/database"
	provider "hitkeep/oauth"
	"hitkeep/server/sites"
)

// Creation stays a human account action; the pending OAuth grant never authorizes writes.
func (h *handler) createSite(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	if h.ctx.OAuthProvider == nil {
		response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	info, err := h.ctx.OAuthProvider.Consent(r.Context(), r, userID)
	if err != nil {
		if errors.Is(err, provider.ErrUnavailable) {
			response(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
			return
		}
		response(w, http.StatusGone, map[string]string{"error": "expired"})
		return
	}
	var request struct {
		TeamID    uuid.UUID `json:"team_id"`
		CSRFToken string    `json:"csrf_token"`
		Domain    string    `json:"domain"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.UnmarshalRead(r.Body, &request); err != nil || request.TeamID == uuid.Nil() || subtle.ConstantTimeCompare([]byte(info.CSRFToken), []byte(request.CSRFToken)) != 1 {
		response(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	teamID := googleuuid.UUID(request.TeamID)
	activeTeam, err := h.ctx.Store.GetActiveTenantID(r.Context(), googleuuid.UUID(userID))
	if err != nil || teamID != activeTeam || info.RegistrationTeamID != uuid.Nil() && info.RegistrationTeamID != request.TeamID {
		response(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	role, err := h.ctx.Store.GetTenantRole(r.Context(), teamID, googleuuid.UUID(userID))
	if err != nil || role != database.TenantRoleOwner && role != database.TenantRoleAdmin {
		response(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	// The canonical handler enforces domain validation, quota locking, tenant mirroring,
	// retention, audit, conversion tracking and webhooks before returning the Site.
	body, err := json.Marshal(map[string]string{"domain": request.Domain})
	if err != nil {
		response(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	sites.CreateSite(h.ctx, teamID)(w, r)
}
