package auth

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/appurl"
	json "hitkeep/jsonapi"
	"hitkeep/server/shared"
)

type nativeManagementMutationKey struct{}

func (h *handler) handleNativeManagementStart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var wire struct {
			api.NativeManagementStart
			Provider json.RawMessage `json:"provider,omitempty"`
			TeamID   json.RawMessage `json:"team_id,omitempty"`
		}
		if !h.nativeJSON(w, r, &wire) {
			return
		}
		input := wire.NativeManagementStart
		if len(wire.Provider) > 0 && (string(wire.Provider) == "null" || json.Unmarshal(wire.Provider, &input.Provider) != nil) || len(wire.TeamID) > 0 && (string(wire.TeamID) == "null" || json.Unmarshal(wire.TeamID, &input.TeamID) != nil) {
			nativeError(w, r, 400, "invalid_request")
			return
		}
		challenge, err := base64.RawURLEncoding.DecodeString(input.CodeChallenge)
		if !shared.NativeAuthClient(input.ClientID, input.RedirectURI) || input.CodeChallengeMethod != "S256" || err != nil || len(challenge) != 32 || base64.RawURLEncoding.EncodeToString(challenge) != input.CodeChallenge || !validNativeOpaque(input.State, 16, 512) {
			nativeError(w, r, 400, "invalid_request")
			return
		}
		target := shared.NativeManagement{Action: input.Action, Provider: input.Provider}
		switch input.Action {
		case "passkey_register":
			if len(wire.Provider) > 0 || len(wire.TeamID) > 0 {
				nativeError(w, r, 400, "invalid_request")
				return
			}
		case "social_link":
			if input.Provider == "" || len(wire.TeamID) > 0 {
				nativeError(w, r, 400, "invalid_request")
				return
			}
		case "search_console_connect":
			target.TeamID, err = uuid.Parse(input.TeamID)
			if err != nil || target.TeamID == uuid.Nil || target.TeamID.String() != input.TeamID || len(wire.Provider) > 0 {
				nativeError(w, r, 400, "invalid_request")
				return
			}
		default:
			nativeError(w, r, 400, "invalid_request")
			return
		}
		id := shared.GetUserIDFromContext(r)
		allowed, err := h.ctx.NativeManagementAuthority(r, id, target)
		if err != nil {
			nativeError(w, r, 503, "temporarily_unavailable")
			return
		}
		if !allowed {
			nativeError(w, r, 403, "access_denied")
			return
		}
		_, security, err := h.nativeAccountSecurity(r, id)
		if err != nil {
			nativeError(w, r, 503, "temporarily_unavailable")
			return
		}
		issuer := shared.NativeAuthIssuer(h.ctx.Config.PublicURL)
		handle, err := h.ctx.AuthState.CreateNativeManagement(input, id, issuer, security, target)
		if err != nil {
			nativeError(w, r, 503, "temporarily_unavailable")
			return
		}
		entry, _ := h.ctx.AuthState.GetNativeAuth(handle)
		shared.WriteJSON(r.Context(), w, 200, api.NativeAuthStarted{AuthorizationURL: issuer + "/api/auth/native/authorize?request=" + url.QueryEscape(handle), ExpiresAt: entry.ExpiresAt})
	}
}

func (h *handler) handleNativeManagementComplete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input api.NativeAuthExchange
		if !h.nativeJSON(w, r, &input) {
			return
		}
		if !shared.NativeAuthClient(input.ClientID, input.RedirectURI) || len(input.Code) != 43 || !validNativeVerifier(input.CodeVerifier) {
			nativeError(w, r, 400, "invalid_grant")
			return
		}
		status, code := 400, "invalid_grant"
		_, ok := h.ctx.AuthState.ConsumeNativeManagement(input, shared.NativeAuthIssuer(h.ctx.Config.PublicURL), func(entry shared.NativeAuthRequest) bool {
			if entry.UserID != shared.GetUserIDFromContext(r) {
				status, code = 403, "access_denied"
				return false
			}
			allowed, err := h.ctx.NativeManagementAuthority(r, entry.UserID, entry.Management)
			if err != nil {
				status, code = 503, "temporarily_unavailable"
				return false
			}
			if !allowed {
				status, code = 403, "access_denied"
				return false
			}
			_, security, err := h.nativeAccountSecurity(r, entry.UserID)
			if err != nil && !errors.Is(err, errNativeAccountMissing) {
				status, code = 503, "temporarily_unavailable"
				return false
			}
			return h.ctx.NativeAuthReady() && err == nil && security == entry.Security && entry.Management.Phase == "succeeded"
		})
		if !ok {
			nativeError(w, r, status, code)
			return
		}
		shared.WriteJSON(r.Context(), w, 200, map[string]string{"status": "ok"})
	}
}

func nativeDestination(entry shared.NativeAuthRequest, handle string) string {
	if entry.Start.Purpose == "management" {
		return shared.NativeManagementReturnPath(handle)
	}
	return shared.NativeAuthReturnPath(handle)
}

func (h *handler) nativeManagementConsent(w http.ResponseWriter, r *http.Request, entry shared.NativeAuthRequest) (*api.NativeManagementConsent, bool) {
	if entry.Start.Purpose != "management" {
		return nil, true
	}
	allowed, err := h.ctx.NativeManagementAuthority(r, entry.UserID, entry.Management)
	if err != nil {
		nativeError(w, r, 503, "temporarily_unavailable")
		return nil, false
	}
	if !allowed {
		nativeError(w, r, 403, "access_denied")
		return nil, false
	}
	out := &api.NativeManagementConsent{Action: entry.Management.Action, Provider: entry.Management.Provider, Phase: entry.Management.Phase}
	if entry.Management.TeamID != uuid.Nil {
		team, err := h.ctx.Store.GetTenant(r.Context(), entry.Management.TeamID)
		if err != nil {
			nativeError(w, r, 503, "temporarily_unavailable")
			return nil, false
		}
		if team == nil {
			nativeError(w, r, 403, "access_denied")
			return nil, false
		}
		out.TeamID, out.TeamName = team.ID.String(), team.Name
	}
	return out, true
}

func nativeCallback(w http.ResponseWriter, r *http.Request, entry shared.NativeAuthRequest, code, failure string) {
	callback, _ := url.Parse(entry.Start.RedirectURI)
	q := url.Values{"state": {entry.Start.State}, "iss": {entry.Issuer}}
	if failure != "" {
		q.Set("error", failure)
	} else {
		q.Set("code", code)
	}
	callback.RawQuery = q.Encode()
	http.Redirect(w, r, callback.String(), http.StatusSeeOther)
}

func (h *handler) nativeManagementDecision(w http.ResponseWriter, r *http.Request, entry shared.NativeAuthRequest, handle, browser, security, decision string) bool {
	if entry.Start.Purpose != "management" {
		return false
	}
	if _, supplied := r.PostForm["remember_me"]; supplied {
		nativeError(w, r, 400, "invalid_request")
		return true
	}
	if _, ok := h.nativeManagementConsent(w, r, entry); !ok {
		return true
	}
	code, ok := h.ctx.AuthState.DecideNativeAuth(handle, browser, r.PostForm.Get("csrf_token"), shared.GetUserIDFromContext(r), security, decision == "approve", false)
	if !ok {
		nativeError(w, r, 403, "access_denied")
		return true
	}
	if decision == "deny" {
		nativeCallback(w, r, entry, "", "access_denied")
		return true
	}
	if code == "" {
		//nolint:gosec // Issuer was matched to this instance and the only destination is its fixed management path with an encoded handle.
		http.Redirect(w, r, appurl.Path(entry.Issuer, shared.NativeManagementReturnPath(handle)), http.StatusSeeOther)
		return true
	}
	nativeCallback(w, r, entry, code, "")
	return true
}

func nativeManagementReturnCandidate(value string) bool {
	return strings.HasPrefix(value, "/auth/native/manage")
}
