//go:build billing

package cloud

import (
	"github.com/google/uuid"
	"hitkeep/config"
	"net/http/httptest"
	"testing"

	"hitkeep/api"
	authcore "hitkeep/auth"
	"hitkeep/database"
	"hitkeep/server/shared"
)

func TestNativeSignupVerificationAcrossBrowsers(t *testing.T) {
	for _, issuer := range []string{"https://cloud.hitkeep.eu", "https://cloud.hitkeep.com", "https://cloud.example.test/prefix"} {
		t.Run(issuer, func(t *testing.T) {
			h, store := setupCloudTestHandler(t)
			defer store.Close()
			h.ctx.Config.PublicURL = issuer
			h.ctx.AuthState = shared.NewAuthStateStore()
			handle, err := h.ctx.AuthState.CreateNativeAuth(api.NativeAuthStart{ClientID: "hitkeep-ios", RedirectURI: shared.NativeIOSCallback, Intent: "signup"}, issuer)
			if err != nil {
				t.Fatal(err)
			}
			token, err := store.CreatePendingSignup(t.Context(), database.PendingSignupEntry{Email: "native-signup@example.test", HashedPassword: "unused", TeamName: "Native signup", Jurisdiction: "EU", Locale: "en", PlanCode: database.CloudPlanFree, BillingInterval: "monthly"})
			if err != nil {
				t.Fatal(err)
			}
			h.ctx.AuthState.SaveNativeEmailReturn(token, shared.NativeAuthReturnPath(handle))
			req := httptest.NewRequest("GET", "/api/cloud/signup/verify?token="+token, nil)
			rr := httptest.NewRecorder()
			h.handleVerifySignup()(rr, req)
			if rr.Code != 303 || rr.Header().Get("Location") != issuer+shared.NativeAuthReturnPath(handle) {
				t.Fatalf("verification %d %s %s", rr.Code, rr.Header().Get("Location"), rr.Body.String())
			}
			user, err := store.GetUserByEmail(t.Context(), "native-signup@example.test")
			if err != nil || user == nil {
				t.Fatalf("account %v", err)
			}
			entry, ok := h.ctx.AuthState.GetNativeAuth(handle)
			if !ok || entry.UserID != user.ID || entry.Approved {
				t.Fatal("verification must bind actual account but never approve")
			}
			found := false
			for _, cookie := range rr.Result().Cookies() {
				if cookie.Name == authcore.CookieName {
					found = true
				}
			}
			if !found {
				t.Fatal("new email browser lacks ordinary cookie")
			}
		})
	}
}

func TestNativeCloudCanonicalHTTPSCookie(t *testing.T) {
	conf := &config.Config{PublicURL: "HTTPS://CLOUD.example.test:443/prefix", JWTSecret: "test-secret"}
	rr := httptest.NewRecorder()
	if err := issueLoginSession(rr, conf, uuid.New()); err != nil {
		t.Fatal(err)
	}
	cookies := rr.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("insecure Cloud cookies: %v", cookies)
	}
	claims, err := authcore.ValidateTokenClaims(cookies[0].Value, conf.JWTSecret, conf.PublicURL)
	if err != nil || claims.Issuer != conf.PublicURL {
		t.Fatalf("raw issuer changed: %v, %v", claims, err)
	}
}
