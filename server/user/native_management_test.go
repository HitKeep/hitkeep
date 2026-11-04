package user

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	appauth "hitkeep/auth"
	json "hitkeep/jsonapi"
	"hitkeep/searchconsole"
	serverauth "hitkeep/server/auth"
	"hitkeep/server/shared"
	"hitkeep/testutil"
)

func TestNativeManagementPasskeyHTTPFlowAndReceiptIsolation(t *testing.T) {
	h, store, id := setupUserSecurityTestEnv(t)
	defer store.Close()
	h.ctx.Config.PublicURL = "https://example.test/prefix"
	mux := http.NewServeMux()
	serverauth.Register(mux, h.ctx)
	Register(mux, h.ctx)
	cookieFor := func(user uuid.UUID) *http.Cookie {
		token, err := appauth.GenerateToken(h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL, user)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Cookie{Name: appauth.CookieName, Value: token}
	}
	nativeCookie := cookieFor(id)
	call := func(method, path string, body any, cookies []*http.Cookie, management string) *httptest.ResponseRecorder {
		var payload string
		contentType := "application/json"
		if form, ok := body.(url.Values); ok {
			payload = form.Encode()
			contentType = "application/x-www-form-urlencoded"
		} else if body != nil {
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			payload = string(raw)
		}
		r := httptest.NewRequest(method, path, strings.NewReader(payload))
		r.Header.Set("Content-Type", contentType)
		r.Header.Set("Origin", "https://example.test")
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		if management != "" {
			r.Header.Set(shared.NativeManagementHeader, management)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	require := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("status=%d want=%d body=%s", w.Code, status, w.Body.String())
		}
	}
	verifier := "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"
	digest := sha256.Sum256([]byte(verifier))
	start := func() string {
		w := call("POST", "/api/auth/native/management/start", api.NativeManagementStart{ClientID: "hitkeep-ios", RedirectURI: shared.NativeIOSCallback, CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:]), CodeChallengeMethod: "S256", State: "random-client-state", Action: "passkey_register"}, []*http.Cookie{nativeCookie}, "")
		require(w, 200)
		if len(w.Result().Cookies()) != 0 {
			t.Fatal("management start issued cookie")
		}
		var out api.NativeAuthStarted
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		u, _ := url.Parse(out.AuthorizationURL)
		return u.Query().Get("request")
	}
	handle := start()
	other := start()
	require(call("GET", "/api/auth/native/consent?request="+handle, nil, []*http.Cookie{nativeCookie}, ""), 400)
	authorize := call("GET", "/api/auth/native/authorize?request="+handle, nil, []*http.Cookie{nativeCookie}, "")
	require(authorize, 303)
	if !strings.Contains(authorize.Header().Get("Location"), "returnUrl=%2Fauth%2Fnative%2Fmanage") {
		t.Fatal("existing cookie did not force narrow fresh login")
	}
	login := call("POST", "/api/login", map[string]any{"email": "user@example.com", "password": "password123", "return_url": shared.NativeManagementReturnPath(handle)}, nil, "")
	require(login, 200)
	browserCookies := login.Result().Cookies()
	consent := func() api.NativeAuthConsent {
		w := call("GET", "/api/auth/native/consent?request="+handle, nil, browserCookies, "")
		require(w, 200)
		browserCookies = append(browserCookies, w.Result().Cookies()...)
		var out api.NativeAuthConsent
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	initial := consent()
	if initial.Management == nil || initial.Management.Action != "passkey_register" || initial.Management.Phase != "pending" {
		t.Fatal("missing exact management consent")
	}
	decision := func(csrf string) *httptest.ResponseRecorder {
		return call("POST", "/api/auth/native/authorize", url.Values{"request": {handle}, "csrf_token": {csrf}, "decision": {"approve"}}, browserCookies, "")
	}
	approved := decision(initial.CSRFToken)
	require(approved, 303)
	if approved.Header().Get("Location") != "https://example.test/prefix"+shared.NativeManagementReturnPath(handle) {
		t.Fatal("approval emitted premature receipt")
	}
	require(call("POST", "/api/user/security/passkeys/register/start", map[string]string{"name": "Wrong browser"}, []*http.Cookie{nativeCookie}, handle), 403)
	begin := call("POST", "/api/user/security/passkeys/register/start", map[string]string{"name": "Browser passkey"}, browserCookies, handle)
	require(begin, 200)
	var options passkeyRegistrationStartResponse
	if err := json.Unmarshal(begin.Body.Bytes(), &options); err != nil {
		t.Fatal(err)
	}
	fixture, err := testutil.NewPasskeyFixture()
	if err != nil {
		t.Fatal(err)
	}
	credential, err := fixture.RegistrationResponse(options.PublicKey.Challenge, "https://example.test", "example.test")
	if err != nil {
		t.Fatal(err)
	}
	finish := call("POST", "/api/user/security/passkeys/register/finish", credential, browserCookies, handle)
	require(finish, 200)
	keys, err := store.ListUserPasskeys(t.Context(), id)
	if err != nil || len(keys) != 1 || keys[0].Name != "Browser passkey" {
		t.Fatal("passkey was not persisted")
	}
	if _, ok := h.ctx.AuthState.GetNativeAuth(other); ok {
		t.Fatal("other pending attempt survived credential commit")
	}
	succeeded := consent()
	if succeeded.Management.Phase != "succeeded" {
		t.Fatal("own receipt lost after credential change")
	}
	callback := decision(succeeded.CSRFToken)
	require(callback, 303)
	u, _ := url.Parse(callback.Header().Get("Location"))
	q := u.Query()
	if u.Scheme+"://"+u.Host+u.Path != shared.NativeIOSCallback || len(q) != 3 || q.Get("iss") != "https://example.test/prefix" || q.Get("state") != "random-client-state" || len(q.Get("code")) != 43 {
		t.Fatalf("callback %s", u)
	}
	exchange := api.NativeAuthExchange{ClientID: "hitkeep-ios", RedirectURI: shared.NativeIOSCallback, Code: q.Get("code"), CodeVerifier: verifier}
	loginMix := call("POST", "/api/auth/native/exchange", exchange, nil, "")
	require(loginMix, 400)
	if len(loginMix.Result().Cookies()) != 0 {
		t.Fatal("receipt minted login")
	}
	missing := call("POST", "/api/auth/native/management/complete", exchange, nil, "")
	require(missing, 401)
	if len(missing.Result().Cookies()) != 0 {
		t.Fatal("missing session issued cookies")
	}
	otherID, err := store.CreateUserWithoutDefaultTenant(t.Context(), "wrong-management@example.test", "unused")
	if err != nil {
		t.Fatal(err)
	}
	wrong := call("POST", "/api/auth/native/management/complete", exchange, []*http.Cookie{cookieFor(otherID)}, "")
	require(wrong, 403)
	for _, want := range []int{200, 400} {
		completed := call("POST", "/api/auth/native/management/complete", exchange, []*http.Cookie{nativeCookie}, "")
		require(completed, want)
		if len(completed.Result().Cookies()) != 0 {
			t.Fatal("management completion issued cookies")
		}
	}
}

func TestNativeManagementSearchConsoleBrowserBindingsAndPrefix(t *testing.T) {
	for _, mode := range []string{"success", "wrong-user", "wrong-browser", "lost-team-role", "changed-credential"} {
		t.Run(mode, func(t *testing.T) {
			h, store, id := setupUserSecurityTestEnv(t)
			defer store.Close()
			h.ctx.Config.PublicURL = "https://example.test/prefix"
			h.ctx.Config.GoogleSearchConsoleClientID = "fixture-client"
			h.ctx.Config.GoogleSearchConsoleClientSecret = "fixture-secret"
			h.ctx.SearchConsole = &fakeSearchConsoleClient{token: searchconsole.Token{AccessToken: "fixture-access", RefreshToken: "fixture-refresh", TokenType: "Bearer", Scope: searchconsole.ReadOnlyScope}}
			teamID, err := store.GetActiveTenantID(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}
			token, err := appauth.GenerateToken(h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL, id)
			if err != nil {
				t.Fatal(err)
			}
			claims, err := appauth.ValidateTokenClaims(token, h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL)
			if err != nil {
				t.Fatal(err)
			}
			_, security, err := h.ctx.NativeAccountSecurity(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}
			target := shared.NativeManagement{Action: "search_console_connect", TeamID: teamID}
			handle, err := h.ctx.AuthState.CreateNativeManagement(api.NativeManagementStart{}, id, "https://example.test/prefix", security, target)
			if err != nil {
				t.Fatal(err)
			}
			if !h.ctx.AuthState.CompleteAccountDeletionPrimary(shared.NativeManagementReturnPath(handle), id, "https://example.test/prefix", claims.ID) {
				t.Fatal("primary")
			}
			csrf, _, ok := h.ctx.AuthState.RenderNativeAuth(handle, "browser", id, security)
			if !ok {
				t.Fatal("consent")
			}
			if _, ok := h.ctx.AuthState.DecideNativeAuth(handle, "browser", csrf, id, security, true, false); !ok {
				t.Fatal("approval")
			}
			request := func(method, path string) *http.Request {
				r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Origin", "https://example.test")
				r.AddCookie(&http.Cookie{Name: appauth.CookieName, Value: token})
				r.AddCookie(&http.Cookie{Name: shared.NativeManagementBrowserCookie, Value: "browser"})
				return r
			}
			mux := http.NewServeMux()
			Register(mux, h.ctx)
			startPath := "/api/user/teams/" + teamID.String() + "/integrations/google-search-console/connect"

			otherTeam := uuid.New()
			if _, err := store.DB().ExecContext(t.Context(), "INSERT INTO tenants(id,name,created_at) VALUES (?, 'Other manageable team', now())", otherTeam); err != nil {
				t.Fatal(err)
			}
			if _, err := store.DB().ExecContext(t.Context(), "INSERT INTO tenant_members(tenant_id,user_id,role) VALUES (?, ?, 'owner')", otherTeam, id); err != nil {
				t.Fatal(err)
			}
			wrongTeam := request("POST", "/api/user/teams/"+otherTeam.String()+"/integrations/google-search-console/connect")
			wrongTeam.Header.Set(shared.NativeManagementHeader, handle)
			deniedTeam := httptest.NewRecorder()
			mux.ServeHTTP(deniedTeam, wrongTeam)
			if deniedTeam.Code != 403 {
				t.Fatalf("wrong authorized team accepted: %d", deniedTeam.Code)
			}
			start := request("POST", startPath)
			start.Header.Set(shared.NativeManagementHeader, handle)
			begun := httptest.NewRecorder()
			mux.ServeHTTP(begun, start)
			if begun.Code != 200 {
				t.Fatalf("start=%d %s", begun.Code, begun.Body.String())
			}
			var response api.GoogleSearchConsoleConnectResponse
			_ = json.Unmarshal(begun.Body.Bytes(), &response)
			authorization, _ := url.Parse(response.AuthURL)
			state := authorization.Query().Get("state")
			if !strings.Contains(response.AuthURL, "https://example.test/prefix/api/integrations/google-search-console/oauth/callback") {
				t.Fatal("provider prefix lost")
			}
			callbackPath := "/api/integrations/google-search-console/oauth/callback?state=" + state + "&code=fixture-code"
			callback := request("GET", callbackPath)
			want := 403
			switch mode {
			case "success":
				want = 303
			case "wrong-user":
				other, err := store.CreateUserWithoutDefaultTenant(t.Context(), "wrong-gsc@example.test", "unused")
				if err != nil {
					t.Fatal(err)
				}
				otherToken, _ := appauth.GenerateToken(h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL, other)
				callback.Header.Set("Cookie", appauth.CookieName+"="+otherToken+"; "+shared.NativeManagementBrowserCookie+"=browser")
				want = 400
			case "wrong-browser":
				callback.Header.Set("Cookie", appauth.CookieName+"="+token+"; "+shared.NativeManagementBrowserCookie+"=other-browser")
			case "lost-team-role":
				if _, err := store.DB().ExecContext(t.Context(), "UPDATE tenant_members SET role = 'member' WHERE tenant_id = ? AND user_id = ?", teamID, id); err != nil {
					t.Fatal(err)
				}
			case "changed-credential":
				if err := store.UpdatePasswordByID(t.Context(), id.String(), "changed"); err != nil {
					t.Fatal(err)
				}
			}
			out := httptest.NewRecorder()
			mux.ServeHTTP(out, callback)
			if out.Code != want {
				t.Fatalf("callback=%d want=%d %s", out.Code, want, out.Body.String())
			}
			connection, err := store.GetGoogleSearchConsoleConnection(t.Context(), teamID)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "success" {
				if connection == nil || !connection.Connected || out.Header().Get("Location") != "https://example.test/prefix"+shared.NativeManagementReturnPath(handle) {
					t.Fatal("persistent success/prefix missing")
				}
				entry, ok := h.ctx.AuthState.GetNativeAuth(handle)
				if !ok || entry.Management.Phase != "succeeded" {
					t.Fatal("success not recorded")
				}
			} else if connection != nil && connection.Connected {
				t.Fatal("denied callback persisted connection")
			}
			replay := httptest.NewRecorder()
			mux.ServeHTTP(replay, request("GET", callbackPath))
			if replay.Code != 400 {
				t.Fatalf("replay=%d", replay.Code)
			}
		})
	}
}

func TestGoogleSearchConsoleOrdinaryCallbackPreservesConfiguredPrefix(t *testing.T) {
	h, store, id := setupUserSecurityTestEnv(t)
	defer store.Close()
	h.ctx.Config.PublicURL = "https://example.test/prefix"
	h.ctx.SearchConsole = &fakeSearchConsoleClient{token: searchconsole.Token{AccessToken: "fixture-access", RefreshToken: "fixture-refresh"}}
	teamID, err := store.GetActiveTenantID(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	state := h.ctx.AuthState.CreateGoogleSearchConsoleOAuthState(id, teamID, "/integration/google-search-console?tab=properties", time.Now().Add(time.Minute))
	r := withTestUser(httptest.NewRequest("GET", "/api/integrations/google-search-console/oauth/callback?state="+state+"&code=fixture", nil), id)
	out := httptest.NewRecorder()
	h.handleGoogleSearchConsoleCallback()(out, r)
	if out.Code != 302 || out.Header().Get("Location") != "https://example.test/prefix/integration/google-search-console?tab=properties" {
		t.Fatalf("prefix return=%d %s", out.Code, out.Header().Get("Location"))
	}
}
