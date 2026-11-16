package admin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/auth"
	"hitkeep/database"
	json "hitkeep/jsonapi"
	"hitkeep/server/shared"
)

func adminDeletionMux(t *testing.T, h *handler, actor, initialOwner uuid.UUID) *http.ServeMux {
	t.Helper()
	if err := h.ctx.Store.UpdateInstanceRole(t.Context(), actor, auth.InstanceOwner, initialOwner); err != nil {
		t.Fatal(err)
	}
	h.ctx.AuthState = shared.NewAuthStateStore()
	mux := http.NewServeMux()
	Register(mux, h.ctx)
	return mux
}

func adminDeletionRequest(t *testing.T, h *handler, actor, target uuid.UUID, force bool, body io.Reader) *http.Request {
	t.Helper()
	path := "/api/admin/users/" + target.String()
	if force {
		path += "?force=true"
	}
	r := httptest.NewRequest("DELETE", path, body)
	r.Header.Set("Content-Type", "application/json")
	token, err := auth.GenerateToken(h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL, actor)
	if err != nil {
		t.Fatal(err)
	}
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	return r
}

func TestAdminUserDeletionRejectsChangedReviewedResources(t *testing.T) {
	h, s, _, _, actor, target := setupAdminTestEnv(t)
	ctx := t.Context()
	mux := adminDeletionMux(t, h, actor, target)
	defaultID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddTeamMember(ctx, defaultID, actor, database.TenantRoleOwner, target); err != nil {
		t.Fatal(err)
	}
	_, aSite := createAdminTestTeamSite(t, s, target, "Reviewed A", "reviewed-a.example.test")
	b, bSite := createAdminTestTeamSite(t, s, target, "Initially shared B", "unreviewed-b.example.test")
	if err := s.AddTeamMember(ctx, b.ID, actor, database.TenantRoleOwner, target); err != nil {
		t.Fatal(err)
	}
	review := httptest.NewRecorder()
	mux.ServeHTTP(review, adminDeletionRequest(t, h, actor, target, false, nil))
	if review.Code != 409 {
		t.Fatalf("review=%d %s", review.Code, review.Body.String())
	}
	var plan struct {
		ConsequenceHash string `json:"consequence_hash"`
	}
	if err := json.Unmarshal(review.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveTeamMember(ctx, b.ID, actor); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"consequence_hash": plan.ConsequenceHash})
	out := httptest.NewRecorder()
	mux.ServeHTTP(out, adminDeletionRequest(t, h, actor, target, true, strings.NewReader(string(body))))
	if out.Code != 409 {
		t.Errorf("changed review executed: %d %s", out.Code, out.Body.String())
	}
	for _, siteID := range []uuid.UUID{aSite.ID, bSite.ID} {
		if site, err := s.GetSiteByID(ctx, siteID); err != nil || site == nil {
			t.Errorf("site destroyed by changed review: %s err=%v", siteID, err)
		}
	}
}

func TestAdminUserDeletionPreflightsDefaultTeam(t *testing.T) {
	h, s, _, _, actor, target := setupAdminTestEnv(t)
	mux := adminDeletionMux(t, h, actor, target)
	site, err := s.CreateSite(t.Context(), target, "default-protected.example.test")
	if err != nil {
		t.Fatal(err)
	}
	out := httptest.NewRecorder()
	mux.ServeHTTP(out, adminDeletionRequest(t, h, actor, target, true, strings.NewReader(`{}`)))
	if out.Code != 409 {
		t.Errorf("default policy status=%d %s", out.Code, out.Body.String())
	}
	if current, err := s.GetSiteByID(t.Context(), site.ID); err != nil || current == nil {
		t.Errorf("default team site deleted before policy check: %+v err=%v", current, err)
	}
}

type adminDeletionBodyGate struct {
	io.Reader
	entered chan<- struct{}
	release <-chan struct{}
	once    bool
}

func (g *adminDeletionBodyGate) Read(p []byte) (int, error) {
	if !g.once {
		g.once = true
		g.entered <- struct{}{}
		<-g.release
	}
	return g.Reader.Read(p)
}

func TestAdminUserDeletionRechecksActorAfterMiddleware(t *testing.T) {
	h, s, _, _, actor, target := setupAdminTestEnv(t)
	ctx := t.Context()
	mux := adminDeletionMux(t, h, actor, target)
	defaultID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddTeamMember(ctx, defaultID, actor, database.TenantRoleOwner, target); err != nil {
		t.Fatal(err)
	}
	entered, release, done := make(chan struct{}, 1), make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	body := &adminDeletionBodyGate{Reader: strings.NewReader(`{}`), entered: entered, release: release}
	out := httptest.NewRecorder()
	req := adminDeletionRequest(t, h, actor, target, false, body)
	go func() { defer close(done); mux.ServeHTTP(out, req) }()
	select {
	case <-entered:
	case <-done:
		t.Fatalf("delete skipped bounded request processing: %d", out.Code)
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not reach body")
	}
	if err := s.UpdateInstanceRole(ctx, actor, auth.InstanceUser, target); err != nil {
		t.Fatal(err)
	}
	// Release after the real permission middleware accepted the previous role.
	unblock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("delete did not finish")
	}
	if out.Code != 403 {
		t.Fatalf("stale owner deleted last owner: %d %s", out.Code, out.Body.String())
	}
	if user, err := s.GetUserByID(ctx, target); err != nil || user == nil {
		t.Fatalf("remaining owner deleted: %+v err=%v", user, err)
	}
}

func TestAdminUserDeletionSerializesMutualOwnerDeletion(t *testing.T) {
	h, s, _, _, a, b := setupAdminTestEnv(t)
	ctx := t.Context()
	mux := adminDeletionMux(t, h, a, b)
	remaining, err := s.CreateUser(ctx, "remaining-team-owner@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddTeamMember(ctx, team, remaining, database.TenantRoleOwner, b); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}, 2), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	outcomes := make(chan int, 2)
	for _, pair := range [][2]uuid.UUID{{a, b}, {b, a}} {
		body := &adminDeletionBodyGate{Reader: strings.NewReader(`{}`), entered: entered, release: release}
		req := adminDeletionRequest(t, h, pair[0], pair[1], false, body)
		go func() { out := httptest.NewRecorder(); mux.ServeHTTP(out, req); outcomes <- out.Code }()
	}
	for range 2 {
		select {
		case <-entered:
		case code := <-outcomes:
			t.Fatalf("request bypassed body barrier: %d", code)
		case <-time.After(5 * time.Second):
			t.Fatal("requests did not pass middleware")
		}
	}
	unblock()
	var success, denied int
	for range 2 {
		select {
		case code := <-outcomes:
			if code == 200 {
				success++
			} else if code == 403 {
				denied++
			} else {
				t.Errorf("unexpected deletion status=%d", code)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("deletion did not finish")
		}
	}
	if success != 1 || denied != 1 {
		t.Fatalf("successful deletes=%d denied=%d", success, denied)
	}
	var owners int
	if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM instance_roles r JOIN users u ON u.id=r.user_id WHERE r.role='owner'`).Scan(&owners); err != nil || owners != 1 {
		t.Fatalf("surviving instance owners=%d err=%v", owners, err)
	}
}

func TestAdminUserDeletionRequiresExactHashAndCommitsReviewedPlan(t *testing.T) {
	h, s, _, _, actor, target := setupAdminTestEnv(t)
	ctx := t.Context()
	mux := adminDeletionMux(t, h, actor, target)
	defaultID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddTeamMember(ctx, defaultID, actor, database.TenantRoleOwner, target); err != nil {
		t.Fatal(err)
	}
	team, site := createAdminTestTeamSite(t, s, target, "Reviewed team", "reviewed.example.test")
	request := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, adminDeletionRequest(t, h, actor, target, true, strings.NewReader(body)))
		return out
	}
	for _, body := range []string{`{"consequence_hash":null}`, `{"consequence_hash":123}`, `{"extra":true}`, `{"consequence_hash":"INVALID"}`, `{"consequence_hash":"` + strings.Repeat("a", 64) + `","consequence_hash":"` + strings.Repeat("a", 64) + `"}`} {
		if out := request(body); out.Code != 400 {
			t.Errorf("malformed request accepted: %s => %d", body, out.Code)
		}
	}
	out := request(`{}`)
	if out.Code != 409 {
		t.Fatalf("missing confirmation=%d %s", out.Code, out.Body.String())
	}
	var review api.AdminDeleteUserBlockedResponse
	if err := json.Unmarshal(out.Body.Bytes(), &review); err != nil {
		t.Fatal(err)
	}
	if review.Code != "delete_confirmation_required" || !review.CanForce || len(review.ConsequenceHash) != 64 || len(review.Teams) != 1 || len(review.Sites) != 1 || review.Sites[0].ID != site.ID || review.Sites[0].UserID != target || review.Sites[0].TeamID != team.ID {
		t.Fatalf("incomplete review=%+v", review)
	}
	for _, change := range []func() error{
		func() error { return s.UpdateTenant(ctx, team.ID, "Renamed reviewed team", "") },
		func() error { return s.UpdateSiteDomain(ctx, site.ID, "renamed.example.test") },
	} {
		oldHash := review.ConsequenceHash
		if err := change(); err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(map[string]string{"consequence_hash": oldHash})
		out = request(string(body))
		if out.Code != 409 {
			t.Fatalf("label change did not invalidate review: %d %s", out.Code, out.Body.String())
		}
		if err := json.Unmarshal(out.Body.Bytes(), &review); err != nil {
			t.Fatal(err)
		}
		if review.Code != "delete_consequences_changed" || review.ConsequenceHash == oldHash {
			t.Fatalf("stale hash retained: %+v", review)
		}
		if current, err := s.GetSiteByID(ctx, site.ID); err != nil || current == nil {
			t.Fatalf("site deleted on stale confirmation: %+v err=%v", current, err)
		}
	}
	paused, err := s.CreateUser(ctx, "paused-team-member@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddTeamMember(ctx, team.ID, paused, database.TenantRoleMember, target); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LinkSocialIdentity(ctx, database.LinkSocialIdentityInput{UserID: paused, Provider: "apple", Subject: "paused-before-archive"}); err != nil {
		t.Fatal(err)
	}
	pausedPlan, err := s.PlanAccountDeletion(ctx, paused, api.AccountDeletionChoices{}, false)
	if err != nil || len(pausedPlan.Blockers) != 0 {
		t.Fatalf("paused plan=%+v err=%v", pausedPlan, err)
	}
	pausedHash, err := database.AccountDeletionConsequenceHash(pausedPlan)
	if err != nil {
		t.Fatal(err)
	}
	attempt := uuid.NewString()
	if err := s.SaveAccountDeletionIntent(ctx, database.AccountDeletionIntent{AttemptID: attempt, UserID: paused, Plan: pausedPlan, ConsequenceHash: pausedHash, ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := h.ctx.TenantStores.DeleteCurrentAccount(ctx, paused, attempt, "paused-jti", pausedHash, func() bool { return true }, nil); !errors.Is(err, database.ErrAccountDeletionUnavailable) {
		t.Fatalf("expected paused provider cleanup: %v", err)
	}
	blockedBody, _ := json.Marshal(map[string]string{"consequence_hash": review.ConsequenceHash})
	blocked := request(string(blockedBody))
	if blocked.Code != 409 {
		t.Fatalf("archive policy discovered after mutation: %d %s", blocked.Code, blocked.Body.String())
	}
	var blockedReview api.AdminDeleteUserBlockedResponse
	if err := json.Unmarshal(blocked.Body.Bytes(), &blockedReview); err != nil || blockedReview.CanForce || blockedReview.Code != "delete_policy_blocked" {
		t.Fatalf("invalid policy review=%+v err=%v", blockedReview, err)
	}
	if current, err := s.GetSiteByID(ctx, site.ID); err != nil || current == nil {
		t.Fatalf("site deleted before member-removal policy check: %+v err=%v", current, err)
	}
	resumed := database.NewTenantStoreManager(s, t.TempDir(), database.WithAccountDeletionProviderRevoker(func(context.Context, uuid.UUID) []api.AccountDeletionProviderRevocation {
		return []api.AccountDeletionProviderRevocation{{Provider: "apple", Status: "revoked"}}
	}))
	t.Cleanup(func() { _ = resumed.Close() })
	if err := resumed.ResumeAccountRemovals(ctx); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"consequence_hash": review.ConsequenceHash})
	out = request(string(body))
	if out.Code != 200 {
		t.Fatalf("matching review not executed: %d %s", out.Code, out.Body.String())
	}
	if current, err := s.GetSiteByID(ctx, site.ID); err != nil || current != nil {
		t.Fatalf("reviewed site retained: %+v err=%v", current, err)
	}
	if current, err := s.GetUserByID(ctx, target); err != nil || current != nil {
		t.Fatalf("reviewed user retained: %+v err=%v", current, err)
	}
	var archived bool
	if err := s.DB().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tenant_archives WHERE tenant_id=?)`, team.ID).Scan(&archived); err != nil || !archived {
		t.Fatalf("reviewed team not archived: %v %v", archived, err)
	}
}
