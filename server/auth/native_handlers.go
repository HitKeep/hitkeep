package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/appurl"
	authcore "hitkeep/auth"
	json "hitkeep/jsonapi"
	"hitkeep/server/shared"
)

const nativeBrowserCookie = "hk_native_browser"

func nativeHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
}

func nativeError(w http.ResponseWriter, r *http.Request, status int, code string) {
	shared.WriteJSON(r.Context(), w, status, map[string]string{"error": code})
}

func (h *handler) registerNativeAuth(mux *http.ServeMux) {
	mux.HandleFunc("/api/auth/native/", func(w http.ResponseWriter, r *http.Request) {
		nativeHeaders(w)
		nativeError(w, r, http.StatusMethodNotAllowed, "invalid_request")
	})
	routes := []struct {
		pattern string
		human   bool
		fn      http.HandlerFunc
	}{
		{"POST /api/auth/native/start", false, h.handleNativeStart()},
		{"GET /api/auth/native/authorize", false, h.handleNativeAuthorize()},
		{"GET /api/auth/native/consent", true, h.handleNativeConsent()},
		{"POST /api/auth/native/authorize", true, h.handleNativeDecision()},
		{"POST /api/auth/native/exchange", false, h.handleNativeExchange()},
	}
	for _, route := range routes {
		wrapped := h.ctx.Handler(shared.HandlerConfig{HumanOnly: route.human, RateLimiter: h.ctx.AuthLimiter}, route.fn)
		mux.HandleFunc(route.pattern, func(w http.ResponseWriter, r *http.Request) {
			nativeHeaders(w)
			if !h.ctx.NativeAuthReady() {
				nativeError(w, r, 503, "temporarily_unavailable")
				return
			}
			if r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Key") != "" || len(r.Header.Values("X-Hitkeep-Agent")) > 0 {
				nativeError(w, r, 403, "access_denied")
				return
			}
			wrapped(w, r)
		})
	}
}

func (h *handler) nativeOrigin(r *http.Request, required bool) bool {
	values := r.Header.Values("Origin")
	if len(values) == 0 {
		return !required
	}
	issuer, _ := url.Parse(shared.NativeAuthIssuer(h.ctx.Config.PublicURL))
	return len(values) == 1 && values[0] == issuer.Scheme+"://"+issuer.Host
}

func (h *handler) nativeJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || !h.nativeOrigin(r, false) || r.URL.RawQuery != "" {
		nativeError(w, r, 400, "invalid_request")
		return false
	}
	if err := json.UnmarshalReadStrict(http.MaxBytesReader(w, r.Body, 4096), out); err != nil {
		nativeError(w, r, 400, "invalid_request")
		return false
	}
	return true
}

func validNativeOpaque(value string, minLen, maxLen int) bool {
	if len(value) < minLen || len(value) > maxLen {
		return false
	}
	for _, r := range value {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

func validNativeVerifier(value string) bool {
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-._~", r)) {
			return false
		}
	}
	return true
}

func (h *handler) handleNativeStart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input api.NativeAuthStart
		if !h.nativeJSON(w, r, &input) {
			return
		}
		challenge, err := base64.RawURLEncoding.DecodeString(input.CodeChallenge)
		if !shared.NativeAuthClient(input.ClientID, input.RedirectURI) || input.CodeChallengeMethod != "S256" || err != nil || len(challenge) != 32 || base64.RawURLEncoding.EncodeToString(challenge) != input.CodeChallenge || !validNativeOpaque(input.State, 16, 512) || (input.Intent != "login" && input.Intent != "signup") {
			nativeError(w, r, 400, "invalid_request")
			return
		}
		if input.Intent == "signup" {
			cloud := h.ctx.CloudStatus()
			if cloud == nil || !cloud.SignupEnabled {
				nativeError(w, r, 403, "access_denied")
				return
			}
		}
		issuer := shared.NativeAuthIssuer(h.ctx.Config.PublicURL)
		handle, err := h.ctx.AuthState.CreateNativeAuth(input, issuer)
		if err != nil {
			nativeError(w, r, 503, "temporarily_unavailable")
			return
		}
		entry, _ := h.ctx.AuthState.GetNativeAuth(handle)
		shared.WriteJSON(r.Context(), w, 200, api.NativeAuthStarted{AuthorizationURL: issuer + "/api/auth/native/authorize?request=" + url.QueryEscape(handle), ExpiresAt: entry.ExpiresAt})
	}
}

func nativeRequestHandle(r *http.Request) string {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(q) != 1 || len(q["request"]) != 1 {
		return ""
	}
	return shared.NativeAuthHandle(shared.NativeAuthReturnPath(q.Get("request")))
}

func (h *handler) handleNativeAuthorize() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handle := nativeRequestHandle(r)
		entry, ok := h.ctx.AuthState.GetNativeAuth(handle)
		if !ok || entry.Issuer != shared.NativeAuthIssuer(h.ctx.Config.PublicURL) {
			nativeError(w, r, 400, "invalid_grant")
			return
		}
		destination := shared.NativeAuthReturnPath(handle)
		if h.userIDFromLogoutRequest(r) == uuid.Nil {
			page := "/login"
			if entry.Start.Intent == "signup" {
				page = "/signup"
			}
			destination = page + "?" + url.Values{"returnUrl": {destination}}.Encode()
		}
		http.Redirect(w, r, appurl.Path(entry.Issuer, destination), http.StatusSeeOther)
	}
}

// nativeAccountSecurity excludes usage timestamps: using a factor must not revoke
// consent, while changing credentials or the account identity must invalidate it.
var errNativeAccountMissing = errors.New("account unavailable")

func (h *handler) nativeAccountSecurity(r *http.Request, id uuid.UUID) (string, string, error) {
	user, err := h.ctx.Store.GetUserByID(r.Context(), id)
	if err != nil {
		return "", "", err
	}
	if user == nil {
		return "", "", errNativeAccountMissing
	}
	secret, enabled, err := h.ctx.Store.GetUserTOTPSecret(r.Context(), id)
	if err != nil {
		return "", "", err
	}
	keys, err := h.ctx.Store.ListUserPasskeys(r.Context(), id)
	if err != nil {
		return "", "", err
	}
	identities, err := h.ctx.Store.ListUserSocialIdentities(r.Context(), id)
	if err != nil {
		return "", "", err
	}
	var material strings.Builder
	material.WriteString(fmt.Sprintf("%s\x00%s\x00%t\x00%s\x00%t", user.Email, user.Password, user.PasswordLoginEnabled, secret, enabled))
	for _, key := range keys {
		material.WriteString("\x00" + key.ID.String())
	}
	for _, identity := range identities {
		material.WriteString("\x00" + identity.Provider + "\x00" + identity.LinkedAt.UTC().String())
	}
	sum := sha256.Sum256([]byte(material.String()))
	return user.Email, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

func (h *handler) handleNativeConsent() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handle := nativeRequestHandle(r)
		id := shared.GetUserIDFromContext(r)
		email, security, err := h.nativeAccountSecurity(r, id)
		if err != nil {
			if errors.Is(err, errNativeAccountMissing) {
				nativeError(w, r, 403, "access_denied")
			} else {
				nativeError(w, r, 503, "temporarily_unavailable")
			}
			return
		}
		browser := ""
		if cookie, err := r.Cookie(nativeBrowserCookie); err == nil && len(cookie.Value) == 43 {
			browser = cookie.Value
		}
		if browser == "" {
			browser = shared.NativeAuthToken()
			issuer, _ := url.Parse(shared.NativeAuthIssuer(h.ctx.Config.PublicURL))
			http.SetCookie(w, &http.Cookie{Name: nativeBrowserCookie, Value: browser, Path: issuer.Path + "/api/auth/native", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: 600})
		}
		csrf, entry, ok := h.ctx.AuthState.RenderNativeAuth(handle, browser, id, security)
		if !ok || entry.Issuer != shared.NativeAuthIssuer(h.ctx.Config.PublicURL) {
			nativeError(w, r, 400, "invalid_grant")
			return
		}
		shared.WriteJSON(r.Context(), w, 200, api.NativeAuthConsent{Request: handle, ClientID: entry.Start.ClientID, Issuer: entry.Issuer, Email: email, CSRFToken: csrf, RememberMe: entry.Start.RememberMe, ExpiresAt: entry.ExpiresAt})
	}
}

func (h *handler) handleNativeDecision() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if !h.nativeOrigin(r, true) || err != nil || media != "application/x-www-form-urlencoded" || r.URL.RawQuery != "" {
			nativeError(w, r, 403, "access_denied")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2048)
		if err := r.ParseForm(); err != nil {
			nativeError(w, r, 400, "invalid_request")
			return
		}
		for key, values := range r.PostForm {
			if len(values) != 1 || (key != "request" && key != "csrf_token" && key != "decision" && key != "remember_me") {
				nativeError(w, r, 400, "invalid_request")
				return
			}
		}
		decision := r.PostForm.Get("decision")
		remember := r.PostForm.Get("remember_me")
		if (decision != "approve" && decision != "deny") || (remember != "true" && remember != "false" && remember != "") {
			nativeError(w, r, 400, "invalid_request")
			return
		}
		handle := r.PostForm.Get("request")
		entry, ok := h.ctx.AuthState.GetNativeAuth(handle)
		if !ok || entry.Issuer != shared.NativeAuthIssuer(h.ctx.Config.PublicURL) {
			nativeError(w, r, 400, "invalid_grant")
			return
		}
		_, security, err := h.nativeAccountSecurity(r, shared.GetUserIDFromContext(r))
		cookie, cookieErr := r.Cookie(nativeBrowserCookie)
		if err != nil && !errors.Is(err, errNativeAccountMissing) {
			nativeError(w, r, 503, "temporarily_unavailable")
			return
		}
		if err != nil || cookieErr != nil {
			nativeError(w, r, 403, "access_denied")
			return
		}
		code, ok := h.ctx.AuthState.DecideNativeAuth(handle, cookie.Value, r.PostForm.Get("csrf_token"), shared.GetUserIDFromContext(r), security, decision == "approve", remember == "true")
		if !ok {
			nativeError(w, r, 403, "access_denied")
			return
		}
		callback, _ := url.Parse(entry.Start.RedirectURI)
		q := url.Values{"state": {entry.Start.State}, "iss": {entry.Issuer}}
		if decision == "approve" {
			q.Set("code", code)
		} else {
			q.Set("error", "access_denied")
		}
		callback.RawQuery = q.Encode()
		http.Redirect(w, r, callback.String(), http.StatusSeeOther)
	}
}

func (h *handler) handleNativeExchange() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input api.NativeAuthExchange
		if !h.nativeJSON(w, r, &input) {
			return
		}
		if !shared.NativeAuthClient(input.ClientID, input.RedirectURI) || len(input.Code) != 43 || !validNativeVerifier(input.CodeVerifier) {
			nativeError(w, r, 400, "invalid_grant")
			return
		}
		var accountError error
		entry, ok := h.ctx.AuthState.ConsumeNativeAuth(input, shared.NativeAuthIssuer(h.ctx.Config.PublicURL), func(entry shared.NativeAuthRequest) bool {
			if !h.ctx.NativeAuthReady() {
				return false
			}
			_, security, err := h.nativeAccountSecurity(r, entry.UserID)
			accountError = err
			return err == nil && security == entry.Security
		})
		if !ok {
			if accountError != nil && !errors.Is(accountError, errNativeAccountMissing) {
				nativeError(w, r, 503, "temporarily_unavailable")
			} else {
				nativeError(w, r, 400, "invalid_grant")
			}
			return
		}
		if !h.ctx.NativeAuthReady() {
			nativeError(w, r, 503, "temporarily_unavailable")
			return
		}
		original := append([]string(nil), w.Header().Values("Set-Cookie")...)
		authcore.ClearCookies(w, true)
		if err := h.issueLoginSession(r.Context(), w, entry.UserID, entry.RememberMe); err != nil {
			w.Header()["Set-Cookie"] = original
			nativeError(w, r, 503, "temporarily_unavailable")
			return
		}
		if entry.RememberMe {
			found := false
			for _, cookie := range (&http.Response{Header: w.Header()}).Cookies() {
				if cookie.Name == authcore.RememberMeCookieName && cookie.Value != "" && cookie.MaxAge >= 0 {
					found = true
				}
			}
			if !found {
				w.Header()["Set-Cookie"] = original
				nativeError(w, r, 503, "temporarily_unavailable")
				return
			}
		}
		shared.WriteJSON(r.Context(), w, 200, map[string]string{"status": "ok"})
	}
}

// The bounded server-owned return path carries no authority: only a verified
// primary method may bind it, and the final cookie subject must match at consent.
type nativeReturnKey struct{}

func withNativeReturnPath(r *http.Request, returnPath string) *http.Request {
	if shared.NativeAuthHandle(returnPath) == "" {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), nativeReturnKey{}, returnPath))
}

func nativeReturnPath(r *http.Request) string {
	value, _ := r.Context().Value(nativeReturnKey{}).(string)
	return value
}

func (h *handler) bindNativeLogin(r *http.Request, w http.ResponseWriter, returnPath string, id uuid.UUID) bool {
	returnPath = nativeSignupReturnPath(returnPath)
	if shared.NativeAuthHandle(returnPath) == "" {
		return true
	}
	nativeHeaders(w)
	if !h.ctx.NativeAuthReady() || !h.ctx.AuthState.BindNativeReturnPath(returnPath, id) {
		nativeError(w, r, 400, "invalid_grant")
		return false
	}
	return true
}

func nativeSignupDestination(returnPath, fallback string) string {
	returnPath = nativeSignupReturnPath(returnPath)
	if shared.NativeAuthHandle(returnPath) != "" {
		return returnPath
	}
	return fallback
}

func nativeSignupReturnPath(returnPath string) string {
	u, err := url.ParseRequestURI(returnPath)
	if err == nil && u.Path == "/signup/social/complete" && !u.IsAbs() && u.Host == "" && len(u.Query()["native"]) == 1 {
		candidate := shared.NativeAuthReturnPath(u.Query().Get("native"))
		if shared.NativeAuthHandle(candidate) != "" {
			return candidate
		}
	}
	return returnPath
}
