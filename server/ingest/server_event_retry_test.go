package ingest

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/auth"
	json "hitkeep/jsonapi"
	"hitkeep/server/shared"
)

func TestServerEventRetryIdentityAndAnonymousDelivery(t *testing.T) {
	producer := &capturingProducer{}
	store, app, _, siteID, token := setupServerIngestTestEnv(t, auth.SiteOwner, func(ctx *shared.Context) { ctx.Producer = producer })
	mux := http.NewServeMux()
	Register(mux, app)
	var first uuid.UUID
	var events []*api.Event
	for range 2 {
		body, err := json.Marshal(map[string]any{"url": "https://example.com/checkout", "timestamp": time.Now().UTC().Format(time.RFC3339), "name": "purchase", "event_id": "woocommerce:purchase:42", "properties": map[string]any{"transaction_id": "42", "value": 10, "currency": "EUR"}})
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, "/api/ingest/server/event", bytes.NewReader(body))
		r.Header.Set("X-API-Key", token)
		w := httptest.NewRecorder()
		producer.messages = nil
		mux.ServeHTTP(w, r)
		if w.Code != http.StatusAccepted {
			t.Fatalf("anonymous order: %d %s", w.Code, w.Body.String())
		}
		var event api.Event
		if err := json.Unmarshal(producer.onlyMessage(t, "events"), &event); err != nil {
			t.Fatal(err)
		}
		if event.ID == uuid.Nil || event.SiteID != siteID {
			t.Fatalf("missing stable identity: %+v", event)
		}
		if first != uuid.Nil && first != event.ID {
			t.Fatal("HTTP retry generated a different event ID")
		}
		first = event.ID
		events = append(events, &event)
	}
	created, err := store.CreateEventsBulkIdempotent(t.Context(), events)
	if err != nil || len(created) != 1 {
		t.Fatalf("retry persisted twice: %d %v", len(created), err)
	}
	created, err = store.CreateEventsBulkIdempotent(t.Context(), events)
	if err != nil || len(created) != 0 {
		t.Fatalf("persisted retry not durable: %d %v", len(created), err)
	}
}

func TestServerEventIdentityValidationAndSiteScope(t *testing.T) {
	for _, invalid := range []string{"has space", "line\nbreak", strings.Repeat("a", 129), "é"} {
		if _, err := serverEventID(uuid.New(), invalid); err == nil {
			t.Fatalf("accepted event_id %q", invalid)
		}
	}
	site := uuid.New()
	a, err := serverEventID(site, "woo:purchase-42")
	if err != nil {
		t.Fatal(err)
	}
	b, err := serverEventID(site, "woo:purchase-42")
	if err != nil {
		t.Fatal(err)
	}
	c, err := serverEventID(uuid.New(), "woo:purchase-42")
	if err != nil {
		t.Fatal(err)
	}
	if a != b || a == c {
		t.Fatal("identity must be stable within one site and isolated across sites")
	}
}
