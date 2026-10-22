package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"uuid"

	"hitkeep/auth"
)

func TestOAuthAuthorizationOptionalParametersAndNativePorts(t *testing.T) {
	for _, tt := range []struct {
		name       string
		registered []string
		redirect   *string
		state      *string
		fetched    bool
	}{
		{name: "omitted state and sole redirect"},
		{name: "short supplied state", state: new("x")},
		{name: "maximum state", state: new(strings.Repeat("s", 1024))},
		{name: "native IPv4 port", registered: []string{"http://127.0.0.1:1234/call%62ack?x=1"}, redirect: new("http://127.0.0.1:51004/call%62ack?x=1")},
		{name: "native IPv6 port", registered: []string{"http://[::1]:1234/callback?x=1"}, redirect: new("http://[::1]:61023/callback?x=1")},
		{name: "configured localhost portless", registered: []string{"http://localhost/callback"}, redirect: new("http://localhost:51004/callback")},
		{name: "CIMD localhost portless", fetched: true, registered: []string{"http://localhost/callback"}, redirect: new("http://localhost:51004/callback")},
		{name: "CIMD IPv4 portless", fetched: true, registered: []string{"http://127.0.0.1/callback"}, redirect: new("http://127.0.0.1:51004/callback")},
		{name: "CIMD IPv6 portless", fetched: true, registered: []string{"http://[::1]/callback"}, redirect: new("http://[::1]:61023/callback")},
		{name: "CIMD IPv4 port change", fetched: true, registered: []string{"http://127.0.0.1:1234/callback?x=1"}, redirect: new("http://127.0.0.1:51004/callback?x=1")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newProviderFixture(t)
			clientID := "test-client"
			var metadataLookups func() int64
			if tt.registered != nil {
				f.options.Clients[0].RedirectURIs = tt.registered
				var err error
				f.p, err = New(f.store, f.options)
				if err != nil {
					t.Fatal(err)
				}
			}
			if tt.fetched {
				metadata := f.options.Clients[0]
				metadata.ID = "https://example.test/client"
				resolver, id, lookups := testResolver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/client" {
						t.Errorf("unexpected metadata fetch: %s", r.URL)
					}
					w.Header().Set("Content-Type", "application/json")
					body, err := json.Marshal(metadata)
					if err != nil {
						t.Error(err)
					}
					_, _ = w.Write(body)
				}))
				f.p.clients = resolver
				clientID = id
				metadataLookups = lookups.Load
			}
			verifier := strings.Repeat("v", 64)
			challenge := sha256.Sum256([]byte(verifier))
			resource := f.p.Resource("api")
			if tt.fetched {
				resource = f.p.Resource("mcp")
			}
			q := url.Values{"response_type": {"code"}, "client_id": {clientID}, "resource": {resource}, "scope": {"sites:read analytics:read"}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}}
			if tt.redirect != nil {
				q.Set("redirect_uri", *tt.redirect)
			}
			if tt.state != nil {
				q.Set("state", *tt.state)
			}
			w := httptest.NewRecorder()
			f.p.Authorize(w, httptest.NewRequest(http.MethodGet, f.p.Issuer()+"/oauth/authorize?"+q.Encode(), nil))
			if w.Code != http.StatusSeeOther || w.Header().Get("Location") != f.p.Issuer()+"/oauth/consent" {
				t.Fatalf("authorization failed: %d %s", w.Code, w.Header().Get("Location"))
			}
			r := httptest.NewRequest(http.MethodPost, f.p.Issuer()+"/api/oauth/consent", nil)
			for _, cookie := range w.Result().Cookies() {
				r.AddCookie(cookie)
			}
			consent, err := f.p.Consent(t.Context(), r, f.userID)
			if err != nil {
				t.Fatal(err)
			}
			authorize := func(_ context.Context, g *auth.OAuthGrant, site uuid.UUID) error {
				if g.UserID != f.userID || g.TeamID != f.teamID || site != f.siteID {
					t.Fatal("consent authority changed")
				}
				return nil
			}
			for _, invalid := range []struct {
				user uuid.UUID
				csrf string
			}{{uuid.NewV7(), consent.CSRFToken}, {f.userID, "wrong"}} {
				if err := f.p.CompleteConsent(httptest.NewRecorder(), r, invalid.user, f.teamID, []uuid.UUID{f.siteID}, "approve", invalid.csrf, authorize); !errors.Is(err, ErrInvalidConsent) {
					t.Fatalf("invalid user/CSRF accepted: %v", err)
				}
			}
			other := r.Clone(t.Context())
			other.Header = r.Header.Clone()
			other.Header.Del("Cookie")
			for _, cookie := range r.Cookies() {
				if cookie.Name == browserCookieName {
					cookie.Value = strings.Repeat("a", 43)
				}
				other.AddCookie(cookie)
			}
			if err := f.p.CompleteConsent(httptest.NewRecorder(), other, f.userID, f.teamID, []uuid.UUID{f.siteID}, "approve", consent.CSRFToken, authorize); !errors.Is(err, ErrInvalidConsent) {
				t.Fatalf("wrong browser accepted: %v", err)
			}
			completed := httptest.NewRecorder()
			if err := f.p.CompleteConsent(completed, r, f.userID, f.teamID, []uuid.UUID{f.siteID}, "approve", consent.CSRFToken, authorize); err != nil {
				t.Fatal(err)
			}
			callback, err := url.Parse(completed.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			if tt.state == nil {
				if callback.Query().Has("state") {
					t.Fatal("invented absent client state")
				}
			} else if callback.Query().Get("state") != *tt.state {
				t.Fatal("client state changed")
			}
			chosen := f.options.Clients[0].RedirectURIs[0]
			if tt.redirect != nil {
				chosen = *tt.redirect
			}
			wanted, err := url.Parse(chosen)
			if err != nil {
				t.Fatal(err)
			}
			if callback.Scheme != wanted.Scheme || callback.Host != wanted.Host || callback.EscapedPath() != wanted.EscapedPath() {
				t.Fatalf("callback changed: %s", callback)
			}
			for key, values := range wanted.Query() {
				if callback.Query().Get(key) != values[0] {
					t.Fatalf("callback query changed: %s", callback)
				}
			}
			form := f.codeForm(callback.Query().Get("code"), verifier)
			form.Set("client_id", clientID)
			form.Set("resource", resource)
			form.Set("redirect_uri", chosen)
			for _, change := range []func(url.Values){
				func(form url.Values) { form.Set("code_verifier", strings.Repeat("x", 64)) },
				func(form url.Values) { form.Set("resource", "https://evil.example/mcp") },
				func(form url.Values) { form.Set("client_id", "other-client") },
				func(form url.Values) {
					form.Set("redirect_uri", strings.Replace(chosen, wanted.Hostname(), "evil.example", 1))
				},
				func(form url.Values) { form.Set("redirect_uri", chosen+"/other") },
				func(form url.Values) { form.Set("redirect_uri", chosen+"?extra=1") },
				func(form url.Values) {
					changed := wanted.Clone()
					changed.Host = net.JoinHostPort(wanted.Hostname(), "51005")
					form.Set("redirect_uri", changed.String())
				},
			} {
				invalid := form.Clone()
				change(invalid)
				if status, result := f.token(t, invalid); status != 400 || result.AccessToken != "" {
					t.Fatalf("invalid native code binding minted a token: status=%d error=%s", status, result.Error)
				}
			}
			status, result := f.token(t, form)
			requireToken(t, status, result)
			grant, err := f.p.Authenticate(t.Context(), result.AccessToken, resource)
			if err != nil || grant == nil || grant.UserID != f.userID || grant.TeamID != f.teamID || len(grant.SiteIDs) != 1 || grant.SiteIDs[0] != f.siteID {
				t.Fatalf("native access token lost consent binding: grant=%+v err=%v", grant, err)
			}
			metadata, err := f.p.clients.resolve(t.Context(), clientID)
			original := f.options.Clients[0]
			if err != nil || !slices.Equal(metadata.RedirectURIs, original.RedirectURIs) || metadata.RegistrationVersion != original.RegistrationVersion || metadata.RegistrationTeamID != original.RegistrationTeamID {
				t.Fatalf("native request mutated registered metadata: metadata=%+v err=%v", metadata, err)
			}
			refresh := f.refreshForm(result.RefreshToken)
			refresh.Set("client_id", clientID)
			refresh.Set("resource", resource)
			status, rotated := f.token(t, refresh)
			requireToken(t, status, rotated)
			if _, err := f.p.Authenticate(t.Context(), rotated.AccessToken, resource); err != nil {
				t.Fatalf("native snapshot pinning broke refresh: %v", err)
			}
			if metadataLookups != nil && metadataLookups() != 1 {
				t.Fatal("native callback caused a metadata refetch or callback destination lookup")
			}
		})
	}
}

func TestOAuthAuthorizationRejectsInvalidOptionalParameters(t *testing.T) {
	for _, tt := range []struct {
		name     string
		change   func(url.Values)
		multiple bool
	}{
		{name: "explicit blank redirect", change: func(q url.Values) { q.Set("redirect_uri", "") }},
		{name: "multiple callbacks omitted", multiple: true, change: func(q url.Values) { q.Del("redirect_uri") }},
		{name: "wrong callback", change: func(q url.Values) { q.Set("redirect_uri", "https://evil.example/callback") }},
		{name: "wrong client", change: func(q url.Values) { q.Set("client_id", "unknown-client") }},
		{name: "wrong resource", change: func(q url.Values) { q.Set("resource", "https://evil.example/api") }},
		{name: "wrong scope", change: func(q url.Values) { q.Set("scope", "admin:write") }},
		{name: "explicit blank state", change: func(q url.Values) { q.Set("state", "") }},
		{name: "oversize state", change: func(q url.Values) { q.Set("state", strings.Repeat("s", 1025)) }},
		{name: "duplicate state", change: func(q url.Values) { q.Set("state", "x"); q.Add("state", "other") }},
		{name: "no PKCE", change: func(q url.Values) { q.Del("code_challenge") }},
		{name: "plain PKCE", change: func(q url.Values) { q.Set("code_challenge_method", "plain") }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newProviderFixture(t)
			if tt.multiple {
				f.options.Clients[0].RedirectURIs = append(f.options.Clients[0].RedirectURIs, "https://client.example/other")
				var err error
				f.p, err = New(f.store, f.options)
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, state := range []string{"", "x"} {
				q := url.Values{"response_type": {"code"}, "client_id": {"test-client"}, "redirect_uri": {"https://client.example/callback?action=oauth"}, "resource": {f.p.Resource("api")}, "scope": {"sites:read"}, "code_challenge_method": {"S256"}, "code_challenge": {strings.Repeat("a", 43)}}
				if state != "" {
					q.Set("state", state)
				}
				tt.change(q)
				w := httptest.NewRecorder()
				f.p.Authorize(w, httptest.NewRequest(http.MethodGet, f.p.Issuer()+"/oauth/authorize?"+q.Encode(), nil))
				if len(w.Result().Cookies()) != 0 || w.Header().Get("Location") == f.p.Issuer()+"/oauth/consent" {
					t.Fatal("invalid authorization created pending browser state")
				}
				if tt.name == "explicit blank redirect" || tt.name == "multiple callbacks omitted" || tt.name == "wrong callback" {
					if w.Code != http.StatusBadRequest || w.Header().Get("Location") != "" {
						t.Fatal("unvalidated callback received redirect")
					}
				}
			}
		})
	}
}

func TestOAuthNativeRedirectChangesOnlyPort(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "[::1]"} {
		for _, declaredPort := range []string{"", ":1234"} {
			declared := "http://" + host + declaredPort + "/call%62ack?x=1&y=2"
			m := ClientMetadata{RedirectURIs: []string{declared}}
			if !redirectAllowed(m, "http://"+host+":51004/call%62ack?x=1&y=2") {
				t.Errorf("declared native host rejected: %s", declared)
			}
			for _, other := range []string{"localhost", "127.0.0.1", "127.0.0.2", "[::1]", "[0:0:0:0:0:0:0:1]"} {
				if other != host && redirectAllowed(m, "http://"+other+":51004/call%62ack?x=1&y=2") {
					t.Errorf("native host aliases equated: %s -> %s", declared, other)
				}
			}
			for _, redirect := range []string{
				"https://" + host + ":51004/call%62ack?x=1&y=2",
				"http://" + host + ":51004/callback?x=1&y=2",
				"http://" + host + ":51004/call%62ack?y=2&x=1",
				"http://" + host + ":51004/call%62ack?x=1&y=3",
				"http://" + host + ":51004/call%62ack?x=1&y=2#",
			} {
				if redirectAllowed(m, redirect) {
					t.Errorf("native callback changed more than port: %s -> %s", declared, redirect)
				}
			}
		}
	}
	m := ClientMetadata{RedirectURIs: []string{"http://127.0.0.1:1234/call%62ack?x=1"}}
	for _, redirect := range []string{"http://127.0.0.1:51004/call%62ack?x=1", "http://127.0.0.1:65535/call%62ack?x=1"} {
		if !redirectAllowed(m, redirect) {
			t.Errorf("native port rejected: %s", redirect)
		}
	}
	for _, redirect := range []string{"http://127.0.0.2:51004/call%62ack?x=1", "http://127.0.0.1:51004/callback?x=1", "http://127.0.0.1:51004/call%62ack?x=2", "https://127.0.0.1:51004/call%62ack?x=1", "http://127.0.0.1:0/call%62ack?x=1", "http://127.0.0.1:65536/call%62ack?x=1", "http://user@127.0.0.1:51004/call%62ack?x=1", "http://127.0.0.1:51004/call%62ack?x=1#", "http://localhost:51004/call%62ack?x=1"} {
		if redirectAllowed(m, redirect) {
			t.Errorf("native callback widened: %s", redirect)
		}
	}
	for _, registered := range []string{"https://127.0.0.1:1234/callback", "https://client.example:1234/callback"} {
		m.RedirectURIs = []string{registered}
		if redirectAllowed(m, strings.Replace(registered, ":1234", ":51004", 1)) {
			t.Errorf("HTTPS callback port widened: %s", registered)
		}
	}
}

func TestOAuthTeamRegistrationKeepsHTTPSOnlyCallbacks(t *testing.T) {
	f := newProviderFixture(t)
	for _, redirect := range []string{"http://localhost/callback", "http://localhost:1234/callback", "http://127.0.0.1/callback", "http://127.0.0.1:1234/callback", "http://[::1]/callback", "http://[::1]:1234/callback"} {
		_, err := f.p.SaveClient(t.Context(), f.userID, f.teamID, ClientRegistration{Name: "Native", RedirectURIs: []string{redirect}}, true)
		if !errors.Is(err, ErrInvalidConsent) {
			t.Errorf("team registration accepted native redirect %q: %v", redirect, err)
		}
	}
	registration, err := f.p.SaveClient(t.Context(), f.userID, f.teamID, ClientRegistration{Name: "Web", RedirectURIs: []string{"https://client.example/callback?x=1"}}, true)
	if err != nil || registration == nil {
		t.Fatalf("existing HTTPS team registration failed: %v", err)
	}
}
