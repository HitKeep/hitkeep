package ingest

import (
	"context"
	stdjson "encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"

	"hitkeep/api"
	"hitkeep/auth"
	"hitkeep/oauth"
	"hitkeep/server/shared"
)

func TestServerIngestPersonalClientLosesCurrentActorRights(t *testing.T) {
	producer := &capturingProducer{}
	store, ctx, userID, siteID, token := setupServerIngestTestEnv(t, auth.SiteOwner, func(ctx *shared.Context) { ctx.Producer = producer })
	if _, err := store.GetAPIClientAuth(t.Context(), token); err != nil {
		t.Fatal(err)
	}
	if err := store.Exec(t.Context(), "UPDATE site_members SET role = 'viewer' WHERE site_id = ? AND user_id = ?", siteID, userID); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, ctx)
	r := httptest.NewRequest(http.MethodPost, "/api/ingest/server/pageview", strings.NewReader(`{"url":"https://example.com/docs","timestamp":"2026-04-03T12:30:45Z","visitor_ip":"198.51.100.22","user_agent":"Mozilla/5.0"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-API-Key", token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("demoted actor: %d, %s", w.Code, w.Body.String())
	}
	if len(producer.messages) != 0 {
		t.Fatal("denied ingest published data")
	}
}

func TestOAuthServerIngestBindsSelectedSiteBeforePublishing(t *testing.T) {
	producer := &capturingProducer{}
	store, app, userID, siteID, _ := setupServerIngestTestEnv(t, auth.SiteOwner, func(ctx *shared.Context) { ctx.Producer = producer })
	p, err := oauth.New(store, oauth.Options{Issuer: "https://example.test", MCPPath: "/mcp", BrowserSigningKey: []byte(strings.Repeat("k", 32)), IsLeader: func() bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	app.OAuthProvider = p
	teamID, err := store.GetSiteTenantID(t.Context(), siteID)
	if err != nil {
		t.Fatal(err)
	}
	grant := &auth.OAuthGrant{ID: uuid.NewV7(), UserID: uuid.UUID(userID), TeamID: uuid.UUID(teamID), SiteIDs: []uuid.UUID{uuid.UUID(siteID)}, Scopes: []string{"ingest:write"}, Audience: p.Resource("api")}
	h := &handler{ctx: app}
	for _, kind := range []string{"pageview", "event"} {
		for _, tc := range []struct {
			name, binding string
			want          int
		}{
			{"missing", "", http.StatusBadRequest},
			{"malformed", `,"site_id":"invalid"`, http.StatusBadRequest},
			{"different destination", fmt.Sprintf(`,"site_id":%q`, uuid.NewV7().String()), http.StatusForbidden},
			{"selected destination", fmt.Sprintf(`,"site_id":%q`, siteID.String()), http.StatusAccepted},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				producer.messages = nil
				body := `{"url":"https://example.com/docs","timestamp":"2026-04-03T12:30:45Z","visitor_ip":"198.51.100.22","user_agent":"Mozilla/5.0","name":"test"` + tc.binding + `}`
				r := httptest.NewRequest(http.MethodPost, "/api/ingest/server/"+kind, strings.NewReader(body))
				r = r.WithContext(context.WithValue(r.Context(), shared.OAuthGrantKey, grant))
				w := httptest.NewRecorder()
				if kind == "event" {
					h.handleServerEventIngestLeader()(w, r)
				} else {
					h.handleServerPageviewIngestLeader()(w, r)
				}
				if w.Code != tc.want {
					t.Fatalf("status = %d, want %d: %s", w.Code, tc.want, w.Body.String())
				}
				if tc.want != http.StatusAccepted {
					if len(producer.messages) != 0 {
						t.Fatal("denied selection published data")
					}
					return
				}
				if len(producer.messages) != 1 {
					t.Fatalf("published %d messages", len(producer.messages))
				}
				if kind == "event" {
					var queued api.Event
					if err := stdjson.Unmarshal(producer.messages[0].body, &queued); err != nil {
						t.Fatal(err)
					}
					if queued.SiteID != siteID {
						t.Fatal("event lost validated site binding")
					}
				} else {
					var queued api.Hit
					if err := stdjson.Unmarshal(producer.messages[0].body, &queued); err != nil {
						t.Fatal(err)
					}
					if queued.SiteID != siteID {
						t.Fatal("hit lost validated site binding")
					}
				}
			})
		}
	}
}
