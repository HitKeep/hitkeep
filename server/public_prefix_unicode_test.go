package server

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"hitkeep/api"
	authcore "hitkeep/auth"
	"hitkeep/entitlements"
	json "hitkeep/jsonapi"
	"hitkeep/server/shared"
)

func TestServerUnicodePublicPrefix(t *testing.T) {
	for index, tc := range []struct {
		configured, canonical string
		requests              []string
	}{
		{"/café/", "/caf%C3%A9", []string{"/café", "/caf%C3%A9", "/caf%c3%a9"}},
		{"/caf%c3%a9", "/caf%C3%A9", []string{"/café", "/caf%C3%A9"}},
		{"/%70refix", "/prefix", []string{"/prefix", "/%70refix"}},
		{"/café", "/cafe%CC%81", []string{"/café", "/cafe%cc%81"}},
		{"/日本", "/%E6%97%A5%E6%9C%AC", []string{"/日本", "/%e6%97%a5%e6%9c%ac"}},
		{"/tools/café", "/tools/caf%C3%A9", []string{"/tools/café", "/%74ools/caf%c3%a9"}},
		{"/📊", "/%F0%9F%93%8A", []string{"/📊", "/%f0%9f%93%8a"}},
		{"/team%20stats", "/team%20stats", []string{"/team%20stats"}},
		{"/discount%25", "/discount%25", []string{"/discount%25"}},
		{"/team!stats", "/team!stats", []string{"/team!stats", "/team%21stats"}},
		{"/team;stats", "/team%3Bstats", []string{"/team;stats", "/team%3bstats"}},
		{"/a&copy;", "/a&copy%3B", []string{"/a&copy;", "/a%26copy%3B"}},
	} {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			conf := testServerConfig(t)
			conf.PublicURL = "https://example.test" + tc.configured
			conf.JWTSecret = "prefix-routing-fixture-secret"
			store := testServerStore(t)
			defer store.Close()
			id, err := store.CreateUserWithoutDefaultTenant(t.Context(), "prefix@example.test", "unused-password-hash")
			if err != nil {
				t.Fatal(err)
			}
			srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil, testServerLogger())
			defer func() { _ = srv.Shutdown(t.Context()) }()
			request := func(method, target, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
				r := httptest.NewRequest(method, "https://example.test"+target, strings.NewReader(body))
				r.Header.Set("Accept", "text/html")
				r.Header.Set("Origin", "https://example.test")
				if method == "POST" {
					r.Header.Set("Content-Type", "application/json")
				}
				for _, c := range cookies {
					r.AddCookie(c)
				}
				out := httptest.NewRecorder()
				srv.httpServer.Handler.ServeHTTP(out, r)
				return out
			}
			issuer := "https://example.test" + tc.canonical
			for _, prefix := range tc.requests {
				web := request("GET", prefix+"/dashboard", "")
				if web.Code != 200 || !strings.Contains(web.Body.String(), `<base href="`+html.EscapeString(tc.canonical+"/")+`" />`) {
					t.Fatalf("dashboard %s status=%d body=%s", prefix, web.Code, web.Body.String())
				}
				status := request("GET", prefix+"/api/status", "")
				if status.Code != 200 {
					t.Fatalf("status %d", status.Code)
				}
				var descriptor struct {
					Native *api.NativeAuthStatus `json:"native_auth"`
				}
				if err := json.Unmarshal(status.Body.Bytes(), &descriptor); err != nil || descriptor.Native == nil || descriptor.Native.Issuer != issuer || !descriptor.Native.Available {
					t.Fatal("native descriptor differs from canonical issuer")
				}
				body := `{"client_id":"hitkeep-ios","redirect_uri":"https://hitkeep.com/auth/native/ios/callback","code_challenge":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","code_challenge_method":"S256","state":"prefix-state-0123456789","intent":"login","remember_me":false}`
				start := request("POST", prefix+"/api/auth/native/start", body)
				if start.Code != 200 {
					t.Fatalf("native start %d %s", start.Code, start.Body.String())
				}
				var started api.NativeAuthStarted
				if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil {
					t.Fatal(err)
				}
				if !strings.HasPrefix(started.AuthorizationURL, issuer+"/api/auth/native/authorize?") {
					t.Fatal("authorization URL lost canonical prefix")
				}
				authorization, _ := url.Parse(started.AuthorizationURL)
				redirect := request("GET", authorization.RequestURI(), "")
				if redirect.Code != 303 || !strings.HasPrefix(redirect.Header().Get("Location"), issuer+"/login?") {
					t.Fatalf("redirect status=%d location=%s", redirect.Code, redirect.Header().Get("Location"))
				}
				token, err := authcore.GenerateToken(conf.JWTSecret, conf.PublicURL, id)
				if err != nil {
					t.Fatal(err)
				}
				claims, err := authcore.ValidateTokenClaims(token, conf.JWTSecret, conf.PublicURL)
				if err != nil || claims.Issuer != conf.PublicURL {
					t.Fatal("JWT issuer changed")
				}
				consent := request("GET", tc.canonical+"/api/auth/native/consent?request="+authorization.Query().Get("request"), "", &http.Cookie{Name: authcore.CookieName, Value: token})
				if consent.Code != 200 {
					t.Fatalf("consent status=%d %s", consent.Code, consent.Body.String())
				}
				found := false
				for _, cookie := range consent.Result().Cookies() {
					if cookie.Name == "hk_native_browser" {
						found = true
						if cookie.Path != tc.canonical+"/api/auth/native" || !cookie.Secure || !cookie.HttpOnly {
							t.Fatal("native nonce cookie lost escaped prefix")
						}
					}
				}
				if !found {
					t.Fatal("nonce cookie missing")
				}
				managementBody := strings.Replace(body, `"intent":"login","remember_me":false`, `"action":"passkey_register"`, 1)
				management := request("POST", prefix+"/api/auth/native/management/start", managementBody, &http.Cookie{Name: authcore.CookieName, Value: token})
				if management.Code != 200 {
					t.Fatalf("management start %d %s", management.Code, management.Body.String())
				}
				if err := json.Unmarshal(management.Body.Bytes(), &started); err != nil {
					t.Fatal(err)
				}
				managementURL, _ := url.Parse(started.AuthorizationURL)
				handle := managementURL.Query().Get("request")
				// Primary authentication is covered separately; this matrix checks its scoped browser cookie.
				if !srv.ctx.AuthState.CompleteAccountDeletionPrimary(shared.NativeManagementReturnPath(handle), id, issuer, claims.ID) {
					t.Fatal("management primary binding failed")
				}
				managementConsent := request("GET", tc.canonical+"/api/auth/native/consent?request="+handle, "", &http.Cookie{Name: authcore.CookieName, Value: token})
				if managementConsent.Code != 200 {
					t.Fatalf("management consent %d %s", managementConsent.Code, managementConsent.Body.String())
				}
				found = false
				for _, cookie := range managementConsent.Result().Cookies() {
					if cookie.Name == shared.NativeManagementBrowserCookie {
						found = true
						if cookie.Path != tc.canonical+"/api" || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
							t.Fatal("management browser cookie lost its escaped prefix or guards")
						}
					}
				}
				if !found {
					t.Fatal("management browser cookie missing")
				}
				page := request("GET", tc.canonical+shared.NativeManagementReturnPath(handle), "")
				if page.Code != 200 || page.Header().Get("Referrer-Policy") != "same-origin" {
					t.Fatal("management document policy lost after prefix stripping")
				}
			}
			for _, bad := range []string{"/%2e%2e/api/status", "/a%2fb/api/status", "/a%5cb/api/status", "/%252e%252e/api/status", "/%00/api/status"} {
				if out := request("GET", bad, ""); out.Code == 200 {
					t.Fatalf("hostile prefix reached route: %s", bad)
				}
			}
			if tc.canonical == "/caf%C3%A9" {
				if out := request("GET", "/cafe%CC%81/api/status", ""); out.Code == 200 {
					t.Fatal("composed and decomposed prefixes were folded")
				}
			}
			if tc.canonical == "/cafe%CC%81" {
				if out := request("GET", "/caf%C3%A9/api/status", ""); out.Code == 200 {
					t.Fatal("decomposed and composed prefixes were folded")
				}
			}
		})
	}
}

func TestStripPublicPrefixPreservesSuffixEscaping(t *testing.T) {
	s := &Server{publicBasePath: "/caf%C3%A9/"}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/items/a/b" || r.URL.EscapedPath() != "/items/a%2Fb" {
			t.Errorf("stripped path=%q raw=%q", r.URL.Path, r.URL.RawPath)
		}
	})
	out := httptest.NewRecorder()
	s.stripPublicBasePath(next).ServeHTTP(out, httptest.NewRequest("GET", "https://example.test/caf%c3%a9/items/a%2Fb", nil))
	if out.Code != 200 {
		t.Fatalf("strip status=%d", out.Code)
	}
	s.publicBasePath = "/"
	out = httptest.NewRecorder()
	s.stripPublicBasePath(next).ServeHTTP(out, httptest.NewRequest("GET", "https://example.test/items/a%2Fb", nil))
	if out.Code != 200 {
		t.Fatalf("root status=%d", out.Code)
	}
}

func TestServerRejectsUnsafePublicPrefix(t *testing.T) {
	for index, prefix := range []string{"/a/../prefix", "/a/./prefix", "//", "/a//b", "/%2Fother", "/a%5cb", "/%252e%252e", "/%00", "/%0a", "/%c0%af", "/bad%xx", "/prefix?query=1", "/prefix#fragment"} {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			conf := testServerConfig(t)
			conf.PublicURL = "https://example.test" + prefix
			if base := normalizePublicBasePath(conf.PublicURL); base != "" {
				t.Fatalf("unsafe prefix normalized to %q", base)
			}
			store := testServerStore(t)
			defer store.Close()
			srv := New(conf, testPublicFS(), store, nil, entitlements.NewProvider(conf), nil, nil, nil, nil, testServerLogger())
			defer func() { _ = srv.Shutdown(t.Context()) }()
			for _, target := range []string{"/api/status", "/prefix/api/status", "/a/api/status"} {
				out := httptest.NewRecorder()
				srv.httpServer.Handler.ServeHTTP(out, httptest.NewRequest("GET", "https://example.test"+target, nil))
				if out.Code != 404 {
					t.Fatalf("unsafe config exposed %s status=%d", target, out.Code)
				}
			}
		})
	}
	s := &Server{publicBasePath: "/safe/base/"}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("hostile encoded prefix reached handler") })
	for _, target := range []string{"/safe%2fbase/api/status", "/safe/base%2fapi/status", "/safe%5cbase/api/status", "/safe/%2e%2e/base/api/status", "/safe/%00base/api/status"} {
		out := httptest.NewRecorder()
		s.stripPublicBasePath(next).ServeHTTP(out, httptest.NewRequest("GET", "https://example.test"+target, nil))
		if out.Code != 404 {
			t.Fatalf("unsafe prefix request status=%d", out.Code)
		}
	}
}
