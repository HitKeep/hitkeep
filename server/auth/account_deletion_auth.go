package auth

import (
	"fmt"
	"mime"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"hitkeep/server/shared"
)

func (h *handler) deletionReauthentication(returnPath string) bool {
	if shared.AccountDeletionHandle(returnPath) != "" {
		return true
	}
	entry, ok := h.ctx.AuthState.GetNativeAuth(shared.NativeAuthHandle(returnPath))
	return ok && entry.Start.Purpose == "account_delete"
}

func (h *handler) validateDeletionSubject(returnPath string, id uuid.UUID) bool {
	if !h.deletionReauthentication(returnPath) {
		return true
	}
	if !h.ctx.NativeAuthReady() {
		return false
	}
	expected, ok := h.ctx.AuthState.AccountDeletionSubject(returnPath, shared.NativeAuthIssuer(h.ctx.Config.PublicURL))
	return ok && expected == id
}

// Only verified-primary completion sites call this, after configured MFA.
func (h *handler) issueVerifiedLoginSession(r *http.Request, w http.ResponseWriter, id uuid.UUID, remember bool, returnPath string) error {
	if !h.validateDeletionSubject(returnPath, id) {
		return fmt.Errorf("reauthentication unavailable")
	}
	original := append([]string(nil), w.Header().Values("Set-Cookie")...)
	jti, err := h.issueLoginSessionWithID(r.Context(), w, id, remember)
	if err != nil {
		return err
	}
	if h.deletionReauthentication(returnPath) && !h.ctx.AuthState.CompleteAccountDeletionPrimary(returnPath, id, shared.NativeAuthIssuer(h.ctx.Config.PublicURL), jti) {
		w.Header()["Set-Cookie"] = original
		return fmt.Errorf("reauthentication unavailable")
	}
	return nil
}

func isDeletionReturnCandidate(returnPath string) bool {
	return strings.HasPrefix(returnPath, "/settings?deletion_attempt=")
}

func (h *handler) deletionAuthRequest(w http.ResponseWriter, r *http.Request, returnPath string) bool {
	if !h.deletionReauthentication(returnPath) && !isDeletionReturnCandidate(returnPath) {
		return true
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || !h.nativeOrigin(r, false) || r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Key") != "" || len(r.Header.Values("X-Hitkeep-Agent")) > 0 {
		nativeError(w, r, 403, "access_denied")
		return false
	}
	return true
}
