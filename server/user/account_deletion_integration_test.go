package user

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"hitkeep/api"
	appauth "hitkeep/auth"
	"hitkeep/database"
	json "hitkeep/jsonapi"
	"hitkeep/security"
	serverauth "hitkeep/server/auth"
	"hitkeep/server/shared"
)

func TestAccountDeletionRoutedPasswordMFAAndCopiedSession(t *testing.T) {
	h, store, departing := setupUserSecurityTestEnv(t)
	t.Cleanup(func() { _ = store.Close() })
	h.ctx.Config.PublicURL = "https://account.example.test"
	ctx := t.Context()
	owner, err := store.CreateUser(ctx, "remaining-integration@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	team, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddTeamMember(ctx, team, owner, database.TenantRoleOwner, departing); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateInstanceRole(ctx, owner, appauth.InstanceOwner, departing); err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, departing, "routed-preserved.example.test")
	if err != nil {
		t.Fatal(err)
	}
	const secret = "JBSWY3DPEHPK3PXP"
	if err := store.EnableUserTOTP(ctx, departing, secret); err != nil {
		t.Fatal(err)
	}
	originalToken, _, err := appauth.GenerateTokenWithDuration(h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL, departing, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	original := &http.Cookie{Name: appauth.CookieName, Value: originalToken}
	mux := http.NewServeMux()
	Register(mux, h.ctx)
	serverauth.Register(mux, h.ctx)
	request := func(method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(encoded))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", h.ctx.Config.PublicURL)
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	prepared := request(http.MethodPost, "/api/user/account/deletion/prepare", api.AccountDeletionChoices{Successors: []api.AccountDeletionSuccessor{{TeamID: team, UserID: owner}}}, original)
	var review api.AccountDeletionPreparation
	if prepared.Code != http.StatusOK {
		t.Fatalf("prepare %d %s", prepared.Code, prepared.Body.String())
	}
	if err := json.Unmarshal(prepared.Body.Bytes(), &review); err != nil || review.AttemptID == "" || review.Reauth.Verified {
		t.Fatalf("review=%+v err=%v", review, err)
	}
	pendingAttempt := h.ctx.AuthState.CreateAccountDeletionAttempt(departing, h.ctx.Config.PublicURL, review.ConsequenceHash, "/settings", time.Now().Add(time.Minute))
	if pendingAttempt == "" {
		t.Fatal("create second pending deletion attempt")
	}
	confirmation := api.AccountDeletionConfirmation{AttemptID: review.AttemptID, ConsequenceHash: review.ConsequenceHash, Confirm: true}
	if w := request(http.MethodDelete, "/api/user/account", confirmation, original); w.Code != http.StatusUnauthorized {
		t.Fatalf("ambient session deleted: %d %s", w.Code, w.Body.String())
	}
	login := request(http.MethodPost, "/api/login", map[string]string{"email": "user@example.com", "password": "password123", "return_url": shared.AccountDeletionReturnPath(review.AttemptID)}, nil)
	var challenge struct {
		Status string `json:"status"`
		Token  string `json:"challenge_token"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &challenge); err != nil || login.Code != http.StatusOK || challenge.Status != "mfa_required" || challenge.Token == "" {
		t.Fatalf("primary %d %s err=%v", login.Code, login.Body.String(), err)
	}
	if len(login.Result().Cookies()) != 0 {
		t.Fatal("primary skipped MFA")
	}
	code, err := security.GenerateCurrentTOTPCode(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	factor := request(http.MethodPost, "/api/auth/mfa/totp/verify", map[string]string{"challenge_token": challenge.Token, "code": code}, nil)
	var fresh *http.Cookie
	for _, cookie := range factor.Result().Cookies() {
		if cookie.Name == appauth.CookieName {
			fresh = cookie
		}
	}
	if factor.Code != http.StatusOK || fresh == nil || fresh.Value == original.Value {
		t.Fatalf("factor %d %s", factor.Code, factor.Body.String())
	}
	preparedExpiry := review.ExpiresAt
	status := request(http.MethodGet, "/api/user/account/deletion/"+review.AttemptID, nil, fresh)
	if err := json.Unmarshal(status.Body.Bytes(), &review); err != nil || status.Code != http.StatusOK || !review.Reauth.Verified {
		t.Fatalf("proof %d %s err=%v", status.Code, status.Body.String(), err)
	}
	if !review.ExpiresAt.Equal(preparedExpiry) {
		t.Fatalf("expiry changed during persistence: prepare=%s status=%s", preparedExpiry.Format(time.RFC3339Nano), review.ExpiresAt.Format(time.RFC3339Nano))
	}
	t.Logf("expiry roundtrip: %s", preparedExpiry.Format(time.RFC3339Nano))
	confirmation.Confirm = false
	if w := request(http.MethodDelete, "/api/user/account", confirmation, fresh); w.Code != http.StatusBadRequest {
		t.Fatalf("missing confirmation accepted: %d", w.Code)
	}
	confirmation.Confirm = true
	deleted := request(http.MethodDelete, "/api/user/account", confirmation, fresh)
	var result api.AccountDeletionResult
	if err := json.Unmarshal(deleted.Body.Bytes(), &result); err != nil || deleted.Code != http.StatusOK || result.Status != "deleted" || result.ProviderRevocations == nil {
		t.Fatalf("delete %d %s err=%v", deleted.Code, deleted.Body.String(), err)
	}
	for _, cookie := range deleted.Result().Cookies() {
		if cookie.Name == appauth.CookieName && (cookie.Value != "" || !cookie.Expires.Before(time.Now())) {
			t.Fatal("session cookie not cleared")
		}
	}
	for _, cookie := range []*http.Cookie{original, fresh} {
		for _, target := range []struct{ method, path string }{{http.MethodGet, "/api/user/profile"}, {http.MethodPost, "/api/auth/session/extend"}, {http.MethodDelete, "/api/user/account"}} {
			if w := request(target.method, target.path, confirmation, cookie); w.Code != http.StatusUnauthorized {
				t.Fatalf("copied session accepted by %s: %d %s", target.path, w.Code, w.Body.String())
			}
		}
	}
	if current, err := store.GetSiteByID(ctx, site.ID); err != nil || current == nil || current.UserID != owner {
		t.Fatalf("shared site=%+v err=%v", current, err)
	}
	if remaining, err := store.GetUserByID(ctx, owner); err != nil || remaining == nil {
		t.Fatalf("remaining owner=%+v err=%v", remaining, err)
	}
	if _, ok := h.ctx.AuthState.AccountDeletionSubject(shared.AccountDeletionReturnPath(pendingAttempt), h.ctx.Config.PublicURL); ok {
		t.Fatal("other pending deletion attempt survived account removal")
	}
	if h.ctx.AuthState.AccountDeletionProofVerified(review.AttemptID, departing, "unused", review.ConsequenceHash) {
		t.Fatal("deleted proof survived")
	}
}
