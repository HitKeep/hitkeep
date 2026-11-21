package ingest

import (
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nsqio/go-nsq"

	"hitkeep/api"
	"hitkeep/database"
	json "hitkeep/jsonapi"
)

type visibleTimeMessageResult struct {
	finished bool
	delay    time.Duration
	backoff  bool
}

func (r *visibleTimeMessageResult) OnFinish(*nsq.Message) { r.finished = true }
func (r *visibleTimeMessageResult) OnTouch(*nsq.Message)  {}
func (r *visibleTimeMessageResult) OnRequeue(_ *nsq.Message, delay time.Duration, backoff bool) {
	r.delay, r.backoff = delay, backoff
}

func TestVisibleTimeConsumerRetriesOnlyMissingPagesWithinBudget(t *testing.T) {
	ctx := t.Context()
	store := setupConsumerStore(t)
	mgr := database.NewTenantStoreManager(store, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })
	userID, err := store.CreateUser(ctx, "visible-time@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	site, err := store.CreateSite(ctx, userID, "visible.example.com")
	if err != nil {
		t.Fatal(err)
	}
	tenantID, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatal(err)
	}
	consumer := NewConsumer(mgr, testBatchLogger(), slog.LevelWarn, nil)
	t.Cleanup(consumer.Stop)

	pageID := uuid.New()
	deliver := func(page, tenant uuid.UUID, ms int, attempts uint16, age time.Duration) visibleTimeMessageResult {
		t.Helper()
		body, err := json.Marshal(api.PageVisibleTimeSample{SiteID: site.ID, TenantID: tenant, PageID: page, VisibleMS: ms})
		if err != nil {
			t.Fatal(err)
		}
		message := nsq.NewMessage(nsq.MessageID{}, body)
		message.Attempts = attempts
		message.Timestamp = time.Now().Add(-age).UnixNano()
		outcome := visibleTimeMessageResult{}
		message.Delegate = &outcome
		if err := consumer.handleVisibleTime(message); err != nil {
			t.Fatal(err)
		}
		return outcome
	}

	if got := deliver(pageID, tenantID, 23, 1, 0); got.finished || got.backoff || got.delay != visibleTimeRetryDelay {
		t.Fatalf("early delivery should retry without backing off the consumer: %+v", got)
	}
	if err := store.CreateHit(ctx, &api.Hit{SiteID: site.ID, PageID: pageID, SessionID: uuid.New(), Path: "/", Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for _, got := range []visibleTimeMessageResult{
		deliver(pageID, tenantID, 23, 2, time.Second),
		deliver(pageID, tenantID, 17, 1, 0),
		deliver(pageID, uuid.New(), 55, 1, 0),
		deliver(uuid.New(), tenantID, 55, visibleTimeAttempts, 0),
		deliver(pageID, tenantID, 55, 1, visibleTimeMaxQueueAge),
	} {
		if !got.finished || got.delay != 0 {
			t.Fatalf("delivery should finish: %+v", got)
		}
	}
	var count, ms int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*), MAX(visible_ms) FROM page_visible_time").Scan(&count, &ms); err != nil || count != 1 || ms != 23 {
		t.Fatalf("durable samples=%d ms=%d err=%v", count, ms, err)
	}
}
