//go:build billing

package sites

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/auth"
	"hitkeep/config"
	"hitkeep/database"
	"hitkeep/server/shared"
	"hitkeep/webhooks"
)

func TestAccountDeletionSerializesHTTPSiteDeleteAndResumesAfterRestart(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	path := filepath.Join(root, "control.db")
	s := database.NewStore(path, database.WithThreads(1))
	if err := s.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	departing, err := s.CreateManagedCloudAccount(ctx, database.CreateManagedCloudAccountInput{Email: "http-departing@example.test", HashedPassword: "hash", TeamName: "Solo", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateManagedCloudAccount(ctx, database.CreateManagedCloudAccountInput{Email: "http-remaining@example.test", HashedPassword: "hash", TeamName: "Unrelated", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTenant(ctx, departing.UserID, "Shared", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddTeamMember(ctx, team.ID, other.UserID, database.TenantRoleOwner, departing.UserID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetActiveTenantID(ctx, departing.UserID, team.ID); err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite(ctx, departing.UserID, "delete-during-verification.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddSiteMember(ctx, site.ID, other.UserID, auth.SiteOwner, departing.UserID); err != nil {
		t.Fatal(err)
	}
	otherSite, err := s.CreateSite(ctx, other.UserID, "unrelated-during-verification.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetActiveTenantID(ctx, other.UserID, team.ID); err != nil {
		t.Fatal(err)
	}
	webhook, _, err := s.CreateWebhook(ctx, &site.ID, api.WebhookInput{Name: "Final site event", URL: "https://webhook.example.test/hook", Enabled: true, Events: []string{webhooks.EventSiteDeleted}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Exec(ctx, `CREATE TABLE account_http_failure_probe (user_id UUID REFERENCES users(id))`); err != nil {
		t.Fatal(err)
	}
	if err := s.Exec(ctx, `INSERT INTO account_http_failure_probe VALUES (?)`, departing.UserID); err != nil {
		t.Fatal(err)
	}
	choices := api.AccountDeletionChoices{Successors: []api.AccountDeletionSuccessor{{TeamID: team.ID, UserID: other.UserID}}, CloseTeamIDs: []uuid.UUID{departing.TenantID}}
	plan, err := s.PlanAccountDeletion(ctx, departing.UserID, choices, true)
	if err != nil || len(plan.Blockers) != 0 {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	hash, err := database.AccountDeletionConsequenceHash(plan)
	if err != nil {
		t.Fatal(err)
	}
	attempt := uuid.NewString()
	if err := s.SaveAccountDeletionIntent(ctx, database.AccountDeletionIntent{AttemptID: attempt, UserID: departing.UserID, Choices: choices, Plan: plan, ConsequenceHash: hash, Cloud: true, ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	manager := database.NewTenantStoreManager(s, root)
	t.Cleanup(func() { _ = manager.Close() })
	if _, _, err := manager.ResolveSiteStore(ctx, site.ID); err != nil {
		t.Fatal(err)
	}
	if role, err := s.GetSiteRole(ctx, other.UserID, site.ID); err != nil || role != auth.SiteOwner {
		t.Fatalf("coowner role=%s err=%v", role, err)
	}
	conf := &config.Config{PublicURL: "https://http.example.test", JWTSecret: "synthetic-http-test-secret", DataPath: root, CloudHosted: true}
	c := &shared.Context{Store: s, TenantStores: manager, Config: conf, AuthState: shared.NewAuthStateStore()}
	mux := http.NewServeMux()
	Register(mux, c)
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	removed := make(chan error, 1)
	go func() {
		removed <- manager.DeleteCurrentAccount(ctx, departing.UserID, attempt, "fresh-jti", hash, func() bool { return true }, func(ctx context.Context, team api.AccountDeletionTeam, _ string) error {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			return s.RecordClosedCloudAccount(ctx, database.ClosedCloudAccount{TeamID: team.ID, CancelAtPeriodEnd: true})
		})
	}()
	select {
	case <-entered:
	case err := <-removed:
		t.Fatalf("removal failed before provider boundary: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("provider boundary not reached")
	}
	token, _, err := auth.GenerateTokenWithDuration(conf.JWTSecret, conf.PublicURL, other.UserID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	deleted := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := httptest.NewRequest(http.MethodDelete, "/api/sites/"+site.ID.String(), nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		deleted <- w
	}()
	var bypassed *httptest.ResponseRecorder
	select {
	case bypassed = <-deleted:
	case <-time.After(100 * time.Millisecond):
	}
	current, readErr := s.GetSiteByID(ctx, site.ID)
	unblock()
	removalErr := <-removed
	if bypassed != nil {
		t.Fatalf("HTTP deletion bypassed final lifecycle boundary: %d %s; removal=%v", bypassed.Code, bypassed.Body.String(), removalErr)
	}
	if readErr != nil || current == nil {
		t.Fatalf("site deleted during provider verification: %+v err=%v", current, readErr)
	}
	if removalErr == nil {
		t.Fatal("expected injected parent FK failure")
	}
	select {
	case w := <-deleted:
		if w.Code != http.StatusOK {
			t.Fatalf("queued coowner delete failed: %d %s", w.Code, w.Body.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("queued site deletion did not finish")
	}
	if current, err := s.GetSiteByID(ctx, site.ID); err != nil || current != nil {
		t.Fatalf("explicit coowner site deletion not applied: %+v err=%v", current, err)
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := database.NewStore(path, database.WithThreads(1))
	if err := restarted.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	if err := restarted.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Exec(ctx, `DROP TABLE account_http_failure_probe`); err != nil {
		t.Fatal(err)
	}
	resumed := database.NewTenantStoreManager(restarted, root)
	t.Cleanup(func() { _ = resumed.Close() })
	if err := resumed.ResumeAccountRemovals(ctx); err != nil {
		t.Fatalf("approved removal stuck after legitimate site deletion: %v", err)
	}
	if result, err := restarted.GetAccountDeletionResult(ctx, attempt, departing.UserID); err != nil || result.Status != "deleted" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	var deliveryID uuid.UUID
	if err := restarted.DB().QueryRowContext(ctx, `SELECT id FROM webhook_deliveries WHERE webhook_id=? AND event_type=?`, webhook.ID, webhooks.EventSiteDeleted).Scan(&deliveryID); err != nil {
		t.Fatal(err)
	}
	if delivery, err := restarted.GetWebhookDelivery(ctx, deliveryID); err != nil || delivery == nil {
		t.Fatalf("final site webhook was not retained: %+v err=%v", delivery, err)
	}
	if user, err := restarted.GetUserByID(ctx, other.UserID); err != nil || user == nil {
		t.Fatalf("other account changed: %+v err=%v", user, err)
	}
	if current, err := restarted.GetSiteByID(ctx, otherSite.ID); err != nil || current == nil || current.UserID != other.UserID {
		t.Fatalf("unrelated site changed: %+v err=%v", current, err)
	}
}
