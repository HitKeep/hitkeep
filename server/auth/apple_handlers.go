package auth

import (
	"context"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"hitkeep/appurl"
	"hitkeep/database"
	json "hitkeep/jsonapi"
	"hitkeep/socialauth"
)

type appleReceiptContextKey struct{}

func (h *handler) handleApplePOSTCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/x-www-form-urlencoded" || r.URL.RawQuery != "" || h.ctx.AuthState == nil {
			writeSocialError(r.Context(), w, http.StatusBadRequest, "social_request_invalid")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		if r.ParseForm() != nil {
			writeSocialError(r.Context(), w, http.StatusBadRequest, "social_request_invalid")
			return
		}
		for key, values := range r.PostForm {
			limit := 4096
			switch key {
			case "state":
				limit = 36
			case "code", "user":
			case "error":
				limit = 128
			case "error_description":
				limit = 1024
			default:
				writeSocialError(r.Context(), w, http.StatusBadRequest, "social_request_invalid")
				return
			}
			if len(values) != 1 || len(values[0]) > limit {
				writeSocialError(r.Context(), w, http.StatusBadRequest, "social_request_invalid")
				return
			}
		}
		state, code, providerError := r.PostForm.Get("state"), r.PostForm.Get("code"), r.PostForm.Get("error")
		if state == "" || (strings.TrimSpace(code) == "") == (strings.TrimSpace(providerError) == "") {
			writeSocialError(r.Context(), w, http.StatusBadRequest, "social_request_invalid")
			return
		}
		if user := r.PostForm.Get("user"); user != "" {
			var object map[string]any
			if json.Unmarshal([]byte(user), &object) != nil || object == nil {
				writeSocialError(r.Context(), w, http.StatusBadRequest, "social_request_invalid")
				return
			}
		}
		receipt, ok := h.ctx.AuthState.CreateSocialPOSTReceipt(state, code, providerError)
		if !ok {
			writeSocialError(r.Context(), w, http.StatusBadRequest, "social_state_invalid")
			return
		}
		http.Redirect(w, r, appurl.Path(h.ctx.Config.PublicURL, "/api/auth/social/apple/resume?receipt="+url.QueryEscape(receipt)), http.StatusSeeOther)
	}
}

func (h *handler) handleAppleResume() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		query := r.URL.Query()
		if len(query) != 1 || len(query["receipt"]) != 1 {
			writeSocialError(r.Context(), w, http.StatusBadRequest, "social_state_invalid")
			return
		}
		receipt, ok := h.ctx.AuthState.GetSocialPOSTReceipt(query.Get("receipt"))
		if !ok || !h.hasSocialStateCookie(r, receipt.State) {
			writeSocialError(r.Context(), w, http.StatusBadRequest, "social_state_invalid")
			return
		}
		// Check continuation availability before consuming the one-use receipt.
		if _, err := h.ctx.OAuthProvider.HasContinuation(r.Context(), r); err != nil {
			http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
			return
		}
		receipt, ok = h.ctx.AuthState.ConsumeSocialPOSTReceipt(query.Get("receipt"))
		if !ok {
			writeSocialError(r.Context(), w, http.StatusBadRequest, "social_state_invalid")
			return
		}
		request := r.Clone(context.WithValue(r.Context(), appleReceiptContextKey{}, true))
		request.URL = r.URL.Clone()
		request.URL.RawQuery = url.Values{"state": {receipt.State}, "code": {receipt.Code}, "error": {receipt.Error}}.Encode()
		request.SetPathValue("provider", socialauth.ProviderApple)
		h.handleSocialCallback().ServeHTTP(w, request)
	}
}

func (h *handler) appleLinkCredential(identity socialauth.Identity) (string, error) {
	if identity.Provider != socialauth.ProviderApple || identity.AppleRefreshToken == "" {
		return "", nil
	}
	return socialauth.SealAppleRefreshToken(h.ctx.Config.SocialAppleTokenEncryptionKey, identity.AppleClientID, identity.Subject, identity.AppleRefreshToken)
}

func (h *handler) revokeAppleIdentity(ctx context.Context, identity *database.SocialIdentity) bool {
	if identity == nil || h.ctx.SocialAuth == nil {
		return false
	}
	cfg, err := socialauth.ConfigForProvider(h.ctx.Config, socialauth.ProviderApple)
	if err != nil {
		return false
	}
	token, err := socialauth.OpenAppleRefreshToken(h.ctx.Config.SocialAppleTokenEncryptionKey, identity.AppleClientID, identity.Subject, identity.AppleRefreshTokenCiphertext)
	return err == nil && h.ctx.SocialAuth.RevokeApple(ctx, cfg, identity.AppleClientID, token) == nil
}
