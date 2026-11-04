package integrations

import (
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"

	"hitkeep/auth"
)

func TestCreateEmbedRequiresOriginAndScopedSite(t *testing.T) {
	h, grant, id := integrationFixture(t)
	h.ctx.Config.JWTSecret = "integration-embed-secret"
	for _, tc := range []struct {
		name, body string
		change     func(*auth.OAuthGrant)
		status     int
	}{
		{"valid", `{"parent_origin":"https://wordpress.example"}`, func(*auth.OAuthGrant) {}, 201},
		{"missing origin", `{}`, func(*auth.OAuthGrant) {}, 400},
		{"unsafe origin", `{"parent_origin":"https://wordpress.example/path"}`, func(*auth.OAuthGrant) {}, 400},
		{"unknown field", `{"parent_origin":"https://wordpress.example","token":"x"}`, func(*auth.OAuthGrant) {}, 400},
		{"wrong site", `{"parent_origin":"https://wordpress.example"}`, func(g *auth.OAuthGrant) { g.SiteIDs = []uuid.UUID{uuid.NewV7()} }, 403},
		{"wrong scope", `{"parent_origin":"https://wordpress.example"}`, func(g *auth.OAuthGrant) { g.Scopes = []string{"sites:read"} }, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := *grant
			tc.change(&g)
			r := integrationRequest(h, &g, "/api/integrations/sites/"+id.String()+"/embed", id)
			r.Method = http.MethodPost
			r.Body = io.NopCloser(strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			h.embed(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d want %d: %s", w.Code, tc.status, w.Body.String())
			}
			if w.Code == 201 {
				var data struct {
					EmbedURL   string `json:"embed_url"`
					EmbedToken string `json:"embed_token"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(data.EmbedURL, data.EmbedToken) || !auth.IsEmbedToken(data.EmbedToken) || !strings.HasPrefix(data.EmbedURL, h.ctx.OAuthProvider.Issuer()+"/embed/"+id.String()+"?launch=") {
					t.Fatalf("unsafe embed response: %s", w.Body.String())
				}
			}
		})
	}
}
