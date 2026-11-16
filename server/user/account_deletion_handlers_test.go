package user

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccountDeletionRejectsCrossOriginAndMixedBearerCookie(t *testing.T) {
	h, store, userID := setupUserSecurityTestEnv(t)
	t.Cleanup(func() { _ = store.Close() })
	for _, test := range []struct {
		name, origin, fetch, authorization string
		status                             int
	}{
		{"cross origin", "https://attacker.invalid", "cross-site", "", http.StatusForbidden},
		{"browser without origin", "", "same-origin", "", http.StatusForbidden},
		{"mixed cookie bearer", "http://localhost:8080", "same-origin", "Bearer copied-credential", http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := withTestUser(httptest.NewRequest(http.MethodDelete, "/api/user/account", strings.NewReader(`{"attempt_id":"attempt","consequence_hash":"hash","confirm":true}`)), userID)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", test.origin)
			req.Header.Set("Sec-Fetch-Site", test.fetch)
			req.Header.Set("Authorization", test.authorization)
			w := httptest.NewRecorder()
			h.handleDeleteCurrentAccount()(w, req)
			if w.Code != test.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if user, err := store.GetUserByID(t.Context(), userID); err != nil || user == nil {
				t.Fatalf("denied request changed account: %v %v", user, err)
			}
		})
	}
}

func TestAccountDeletionReauthPathStaysOnLoginAndBindsOpaqueAttempt(t *testing.T) {
	got := accountDeletionReauthPath("a&returnUrl=https://attacker.invalid")
	if !strings.HasPrefix(got, "/login?returnUrl=%2Fsettings%3Fdeletion_attempt%3D") || strings.Contains(got, "https://") {
		t.Fatalf("unsafe path: %s", got)
	}
}
