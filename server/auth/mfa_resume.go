package auth

import (
	"net/http"
	"net/url"

	"github.com/google/uuid"

	"hitkeep/appurl"
	authcore "hitkeep/auth"
	"hitkeep/server/shared"
)

const mfaResumeCookie = "hk_mfa_resume"

func (h *handler) redirectDeletionMFA(w http.ResponseWriter, r *http.Request, returnPath string, response loginResponse) bool {
	id, err := uuid.Parse(response.ChallengeToken)
	if err != nil {
		return false
	}
	browser := shared.NativeAuthToken()
	if !h.ctx.AuthState.BindMFAResume(id, browser, response.Factors, response.Passkey) {
		return false
	}
	issuer, _ := url.Parse(shared.AccountDeletionIssuer(h.ctx.Config.PublicURL))
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure follows the configured browser issuer; HTTP browser reauthentication is intentionally supported.
		Name: mfaResumeCookie, Value: browser, Path: issuer.Path + "/api/auth/mfa", HttpOnly: true, Secure: authcore.CookieSecure(h.ctx.Config.PublicURL), SameSite: http.SameSiteStrictMode, MaxAge: 300})
	destination := "/login?" + url.Values{"returnUrl": {returnPath}}.Encode() + "#mfa_challenge=" + url.QueryEscape(response.ChallengeToken)
	http.Redirect(w, r, appurl.Path(h.ctx.Config.PublicURL, destination), http.StatusSeeOther)
	return true
}

func (h *handler) handleMFAResume() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nativeHeaders(w)
		if !h.ctx.AccountDeletionReady() {
			nativeError(w, r, 503, "temporarily_unavailable")
			return
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Key") != "" || len(r.Header.Values("X-Hitkeep-Agent")) > 0 || !h.nativeOrigin(r, true) {
			nativeError(w, r, 403, "access_denied")
			return
		}
		var input struct {
			ChallengeID string `json:"challenge_id"`
		}
		if !h.nativeJSON(w, r, &input) {
			return
		}
		id, err := uuid.Parse(input.ChallengeID)
		if err != nil || id.String() != input.ChallengeID {
			nativeError(w, r, 400, "invalid_grant")
			return
		}
		cookie, err := r.Cookie(mfaResumeCookie)
		if err != nil {
			nativeError(w, r, 400, "invalid_grant")
			return
		}
		challenge, ok := h.ctx.AuthState.GetPasskeyLoginChallenge(id)
		if !ok || !h.deletionReauthentication(challenge.NativeReturnPath) || !h.validateDeletionSubject(challenge.NativeReturnPath, challenge.UserID) {
			nativeError(w, r, 400, "invalid_grant")
			return
		}
		factors, passkey, ok := h.ctx.AuthState.MFAResume(id, cookie.Value)
		if !ok {
			nativeError(w, r, 400, "invalid_grant")
			return
		}
		available, err := h.ctx.HumanSessionUserAvailable(r.Context(), challenge.UserID)
		if err != nil {
			nativeError(w, r, 503, "temporarily_unavailable")
			return
		}
		if !available {
			nativeError(w, r, 400, "invalid_grant")
			return
		}
		shared.WriteJSON(r.Context(), w, 200, loginResponse{Status: "mfa_required", ChallengeToken: input.ChallengeID, Factors: factors, Passkey: passkey})
	}
}
