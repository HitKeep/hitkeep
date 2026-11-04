package user

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/auth"
	"hitkeep/database"
	json "hitkeep/jsonapi"
	"hitkeep/server/shared"
)

// Auth owns creation and completion of fresh proof at actual login/MFA sites.
type accountDeletionProofState interface {
	CreateAccountDeletionAttempt(uuid.UUID, string, string, string, time.Time) string
	ConsumeAccountDeletionProof(string, uuid.UUID, string, string) bool
	AccountDeletionProofVerified(string, uuid.UUID, string, string) bool
}

func (h *handler) registerAccountDeletion(mux *http.ServeMux) {
	config := shared.HandlerConfig{RequireAuth: true, HumanOnly: true, RateLimiter: h.ctx.AuthLimiter}
	mux.HandleFunc("POST /api/user/account/deletion/prepare", h.ctx.Handler(config, h.handlePrepareAccountDeletion()))
	mux.HandleFunc("GET /api/user/account/deletion/{attempt_id}", h.ctx.Handler(config, h.handleAccountDeletionStatus()))
	mux.HandleFunc("DELETE /api/user/account", h.ctx.Handler(config, h.handleDeleteCurrentAccount()))
}

func (h *handler) accountDeletionSession(w http.ResponseWriter, r *http.Request, mutation bool) (uuid.UUID, string, accountDeletionProofState, bool) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Header.Get("Authorization") != "" || r.Header.Get("X-Hitkeep-Embed-Parent") != "" || r.Context().Value(shared.APIClientAuthKey) != nil {
		accountDeletionError(w, r, http.StatusUnauthorized, "human_session_required")
		return uuid.Nil, "", nil, false
	}
	if h.ctx == nil || h.ctx.Store == nil || h.ctx.TenantStores == nil || h.ctx.Config == nil || (h.ctx.Cluster != nil && !h.ctx.Cluster.IsLeader()) {
		accountDeletionError(w, r, http.StatusServiceUnavailable, "unavailable")
		return uuid.Nil, "", nil, false
	}
	if mutation {
		public, err := url.Parse(h.ctx.Config.PublicURL)
		origin := r.Header.Get("Origin")
		fetchSite := r.Header.Get("Sec-Fetch-Site")
		if err != nil || (origin != "" && origin != public.Scheme+"://"+public.Host) || (fetchSite != "" && fetchSite != "same-origin") || (fetchSite != "" && origin == "") || r.URL.RawQuery != "" {
			accountDeletionError(w, r, http.StatusForbidden, "origin_required")
			return uuid.Nil, "", nil, false
		}
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			accountDeletionError(w, r, http.StatusBadRequest, "invalid_request")
			return uuid.Nil, "", nil, false
		}
	}
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil {
		accountDeletionError(w, r, http.StatusUnauthorized, "human_session_required")
		return uuid.Nil, "", nil, false
	}
	claims, err := auth.ValidateTokenClaims(cookie.Value, h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL)
	userID := shared.GetUserIDFromContext(r)
	if err != nil || claims == nil || claims.UserID != userID || userID == uuid.Nil || claims.ID == "" {
		accountDeletionError(w, r, http.StatusUnauthorized, "human_session_required")
		return uuid.Nil, "", nil, false
	}
	proof, ok := any(h.ctx.AuthState).(accountDeletionProofState)
	if !ok || h.ctx.AuthState == nil {
		accountDeletionError(w, r, http.StatusServiceUnavailable, "reauth_unavailable")
		return uuid.Nil, "", nil, false
	}
	return userID, claims.ID, proof, true
}

func (h *handler) handlePrepareAccountDeletion() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _, proof, ok := h.accountDeletionSession(w, r, true)
		if !ok {
			return
		}
		var choices api.AccountDeletionChoices
		if err := json.UnmarshalReadStrict(http.MaxBytesReader(w, r.Body, 32<<10), &choices); err != nil || len(choices.Successors) > 512 || len(choices.CloseTeamIDs) > 512 {
			accountDeletionError(w, r, http.StatusBadRequest, "invalid_request")
			return
		}
		plan, err := h.ctx.Store.PlanAccountDeletion(r.Context(), userID, choices, h.ctx.Config.CloudHosted)
		if err != nil {
			accountDeletionError(w, r, http.StatusConflict, "invalid_selection")
			return
		}
		response := api.AccountDeletionPreparation{AccountDeletionPlan: plan, Purpose: "delete-current-account", Phase: "blocked"}
		response.Retention.BillingRecords = true
		response.Retention.Cancellation = "period_end"
		if len(plan.Blockers) != 0 {
			shared.WriteJSON(r.Context(), w, http.StatusOK, response)
			return
		}
		hash, err := database.AccountDeletionConsequenceHash(plan)
		if err != nil {
			accountDeletionError(w, r, http.StatusServiceUnavailable, "unavailable")
			return
		}
		expiry := time.Now().UTC().Add(10 * time.Minute)
		attempt := proof.CreateAccountDeletionAttempt(userID, h.ctx.Config.PublicURL, hash, "/settings", expiry)
		if attempt == "" {
			accountDeletionError(w, r, http.StatusServiceUnavailable, "reauth_unavailable")
			return
		}
		if err := h.ctx.Store.SaveAccountDeletionIntent(r.Context(), database.AccountDeletionIntent{AttemptID: attempt, UserID: userID, ConsequenceHash: hash, Choices: choices, Plan: plan, Cloud: h.ctx.Config.CloudHosted, ExpiresAt: expiry}); err != nil {
			accountDeletionError(w, r, http.StatusServiceUnavailable, "unavailable")
			return
		}
		response.AttemptID = attempt
		response.ConsequenceHash = hash
		response.ExpiresAt = expiry
		response.Phase = "prepared"
		response.Reauth.ReturnPath = accountDeletionReauthPath(attempt)
		response.Reauth.Methods = h.accountDeletionMethods(r.Context(), userID)
		shared.WriteJSON(r.Context(), w, http.StatusOK, response)
	}
}

func accountDeletionReauthPath(attempt string) string {
	return "/login?" + url.Values{"returnUrl": {"/settings?" + url.Values{"deletion_attempt": {attempt}}.Encode()}}.Encode()
}

func (h *handler) accountDeletionMethods(ctx context.Context, userID uuid.UUID) []string {
	methods := []string{}
	status, err := h.getUserSecurityStatus(ctx, userID)
	if err != nil {
		return methods
	}
	if status.PasswordLoginEnabled {
		methods = append(methods, "password")
	}
	if len(status.Passkeys) > 0 {
		methods = append(methods, "passkey")
	}
	for _, identity := range status.SocialIdentities {
		methods = append(methods, identity.Provider)
	}
	var hasSSO bool
	if err := h.ctx.Store.DB().QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM sso_identities si JOIN team_sso_configs cfg ON cfg.tenant_id=si.tenant_id WHERE si.user_id=? AND cfg.enabled)`, userID).Scan(&hasSSO); err == nil && hasSSO {
		methods = append(methods, "sso")
	}
	return methods
}

func (h *handler) handleAccountDeletionStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, sessionID, proof, ok := h.accountDeletionSession(w, r, false)
		if !ok {
			return
		}
		intent, err := h.ctx.Store.GetAccountDeletionIntent(r.Context(), r.PathValue("attempt_id"), userID)
		if err != nil {
			accountDeletionError(w, r, http.StatusNotFound, "attempt_not_found")
			return
		}
		response := api.AccountDeletionPreparation{AccountDeletionPlan: intent.Plan, AttemptID: intent.AttemptID, ConsequenceHash: intent.ConsequenceHash, ExpiresAt: intent.ExpiresAt, Purpose: "delete-current-account", Phase: intent.Phase}
		response.Reauth.ReturnPath = accountDeletionReauthPath(intent.AttemptID)
		response.Reauth.Methods = h.accountDeletionMethods(r.Context(), userID)
		response.Reauth.Verified = proof.AccountDeletionProofVerified(intent.AttemptID, userID, sessionID, intent.ConsequenceHash) || (intent.Phase == "authorized" && intent.SessionID == sessionID && time.Now().Before(intent.ExpiresAt))
		response.Retention.BillingRecords = true
		response.Retention.Cancellation = "period_end"
		shared.WriteJSON(r.Context(), w, http.StatusOK, response)
	}
}

func (h *handler) handleDeleteCurrentAccount() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, sessionID, proof, ok := h.accountDeletionSession(w, r, true)
		if !ok {
			return
		}
		var confirmation api.AccountDeletionConfirmation
		if err := json.UnmarshalReadStrict(http.MaxBytesReader(w, r.Body, 4<<10), &confirmation); err != nil || !confirmation.Confirm || confirmation.AttemptID == "" || len(confirmation.ConsequenceHash) != 64 {
			accountDeletionError(w, r, http.StatusBadRequest, "explicit_confirmation_required")
			return
		}
		err := h.ctx.TenantStores.DeleteCurrentAccount(r.Context(), userID, confirmation.AttemptID, sessionID, confirmation.ConsequenceHash, func() bool {
			return proof.ConsumeAccountDeletionProof(confirmation.AttemptID, userID, sessionID, confirmation.ConsequenceHash)
		}, func(ctx context.Context, team api.AccountDeletionTeam, attempt string) error {
			return h.stopAccountDeletionRenewals(ctx, team, attempt)
		})
		if err != nil {
			switch {
			case errors.Is(err, database.ErrAccountDeletionProof):
				accountDeletionError(w, r, http.StatusUnauthorized, "reauth_required")
			case errors.Is(err, database.ErrAccountDeletionChanged), errors.Is(err, database.ErrAccountDeletionBlocked):
				accountDeletionError(w, r, http.StatusConflict, "consequences_changed")
			default:
				accountDeletionError(w, r, http.StatusServiceUnavailable, "closure_retry_required")
			}
			return
		}
		h.ctx.AuthState.ClearUser(userID)
		auth.ClearCookies(w, strings.HasPrefix(h.ctx.Config.PublicURL, "https://"))
		result, err := h.ctx.Store.GetAccountDeletionResult(r.Context(), confirmation.AttemptID, userID)
		if err != nil {
			accountDeletionError(w, r, http.StatusServiceUnavailable, "closure_retry_required")
			return
		}
		shared.WriteJSON(r.Context(), w, http.StatusOK, result)
	}
}

func accountDeletionError(w http.ResponseWriter, r *http.Request, status int, code string) {
	shared.WriteJSON(r.Context(), w, status, map[string]any{"error": map[string]string{"code": code, "message": code}})
}
