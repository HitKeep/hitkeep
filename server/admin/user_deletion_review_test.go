package admin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestTeamOwnerSurvivesAnotherOwnersInterruptedRemoval(t *testing.T) {
	for _, operation := range []string{"admin", "personal", "leave", "demote", "remove"} {
		t.Run(operation, func(t *testing.T) {
			h, s, _, _, actor, survivor := setupAdminTestEnv(t)
			ctx := t.Context()
			mux := adminDeletionMux(t, h, actor, survivor)
			defaultID, err := s.GetDefaultTenantID(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.AddTeamMember(ctx, defaultID, actor, database.TenantRoleOwner, survivor); err != nil {
				t.Fatal(err)
			}
			if err := s.AddTeamMember(ctx, defaultID, survivor, database.TenantRoleMember, actor); err != nil {
				t.Fatal(err)
			}
			removing, err := s.CreateUser(ctx, "interrupted-owner@example.test", "hash")
			if err != nil {
				t.Fatal(err)
			}
			team, err := s.CreateTenant(ctx, survivor, "Retained shared team", "")
			if err != nil {
				t.Fatal(err)
			}
			if err := s.AddTeamMember(ctx, team.ID, removing, database.TenantRoleOwner, survivor); err != nil {
				t.Fatal(err)
			}
			if err := s.AddTeamMember(ctx, team.ID, actor, database.TenantRoleMember, survivor); err != nil {
				t.Fatal(err)
			}
			if err := s.SetActiveTenantID(ctx, actor, team.ID); err != nil {
				t.Fatal(err)
			}
			site, err := s.CreateSite(ctx, actor, "retained-live-owner.example.test")
			if err != nil {
				t.Fatal(err)
			}
			failingTeam, failingSite := createAdminTestTeamSite(t, s, removing, "Sync failure team", "interrupted-sync.example.test")
			if err := s.AddTeamMember(ctx, failingTeam.ID, actor, database.TenantRoleOwner, removing); err != nil {
				t.Fatal(err)
			}
			prepare := func(user uuid.UUID, choices api.AccountDeletionChoices) (string, string) {
				t.Helper()
				plan, err := s.PlanAccountDeletion(ctx, user, choices, false)
				if err != nil || len(plan.Blockers) != 0 {
					t.Fatalf("prepare=%+v err=%v", plan, err)
				}
				hash, err := database.AccountDeletionConsequenceHash(plan)
				if err != nil {
					t.Fatal(err)
				}
				attempt := uuid.NewString()
				if err := s.SaveAccountDeletionIntent(ctx, database.AccountDeletionIntent{AttemptID: attempt, UserID: user, Choices: choices, Plan: plan, ConsequenceHash: hash, ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
					t.Fatal(err)
				}
				return attempt, hash
			}
			survivorAttempt, survivorHash := prepare(survivor, api.AccountDeletionChoices{})
			attempt, hash := prepare(removing, api.AccountDeletionChoices{Successors: []api.AccountDeletionSuccessor{{TeamID: failingTeam.ID, UserID: actor}}})
			dataPath := filepath.Join(t.TempDir(), "blocked-data-directory")
			if err := os.WriteFile(dataPath, []byte("block tenant directory creation"), 0o600); err != nil {
				t.Fatal(err)
			}
			manager := database.NewTenantStoreManager(s, dataPath)
			t.Cleanup(func() { _ = manager.Close() })
			h.ctx.TenantStores = manager
			if err := manager.DeleteCurrentAccount(ctx, removing, attempt, "removing-jti", hash, func() bool { return true }, nil); err == nil {
				t.Fatal("expected real tenant sync failure")
			}
			if started, err := s.AccountRemovalStarted(ctx, removing); err != nil || !started {
				t.Fatalf("removing=%v err=%v", started, err)
			}
			if current, err := s.GetSiteByID(ctx, failingSite.ID); err != nil || current == nil || current.UserID != actor {
				t.Fatalf("reparenting did not precede sync failure: %+v err=%v", current, err)
			}
			if members, err := s.ListTeamMembers(ctx, team.ID); err != nil || len(members) != 3 {
				t.Fatalf("pending cleanup memberships lost: %+v err=%v", members, err)
			}
			if owners, err := s.CountTeamOwners(ctx, team.ID); err != nil || owners != 1 {
				t.Errorf("removing peer counted live: owners=%d err=%v", owners, err)
			}
			plan, err := s.PlanAccountDeletion(ctx, survivor, api.AccountDeletionChoices{Successors: []api.AccountDeletionSuccessor{{TeamID: team.ID, UserID: removing}}}, false)
			if err != nil {
				t.Fatal(err)
			}
			blocked := false
			for _, blocker := range plan.Blockers {
				blocked = blocked || blocker.TeamID == team.ID && blocker.Code == "invalid_successor"
			}
			if !blocked {
				t.Errorf("removing peer accepted as successor: %+v", plan)
			}
			switch operation {
			case "admin":
				out := httptest.NewRecorder()
				mux.ServeHTTP(out, adminDeletionRequest(t, h, actor, survivor, false, nil))
				var review api.AdminDeleteUserBlockedResponse
				if err := json.Unmarshal(out.Body.Bytes(), &review); err != nil || out.Code != 409 || review.CanForce || len(review.Teams) != 1 || review.Teams[0].ID != team.ID || len(review.Sites) != 1 || review.Sites[0].ID != site.ID {
					t.Errorf("live sole owner deletion was not blocked: status=%d body=%s err=%v", out.Code, out.Body, err)
				}
			case "personal":
				if err := manager.DeleteCurrentAccount(ctx, survivor, survivorAttempt, "survivor-jti", survivorHash, func() bool { return true }, nil); !errors.Is(err, database.ErrAccountDeletionChanged) {
					t.Errorf("prepared deletion relied on removing owner: %v", err)
				}
			case "leave":
				if _, err := s.LeaveTeam(ctx, team.ID, survivor); !errors.Is(err, database.ErrTeamLastOwner) {
					t.Errorf("last live owner left: %v", err)
				}
			case "demote":
				if err := s.AddTeamMember(ctx, team.ID, survivor, database.TenantRoleMember, survivor); !errors.Is(err, database.ErrTeamLastOwner) {
					t.Errorf("last live owner demoted: %v", err)
				}
			case "remove":
				if err := s.RemoveTeamMember(ctx, team.ID, survivor); !errors.Is(err, database.ErrTeamLastOwner) {
					t.Errorf("last live owner removed: %v", err)
				}
			}
			if err := os.Remove(dataPath); err != nil {
				t.Fatal(err)
			}
			if err := manager.ResumeAccountRemovals(ctx); err != nil {
				t.Fatal(err)
			}
			if user, err := s.GetUserByID(ctx, removing); err != nil || user != nil {
				t.Errorf("interrupted removal did not resume: %+v err=%v", user, err)
			}
			if role, err := s.GetTenantRole(ctx, team.ID, survivor); err != nil || role != database.TenantRoleOwner {
				t.Errorf("surviving team lost owner: role=%q err=%v", role, err)
			}
			if current, err := s.GetSiteByID(ctx, site.ID); err != nil || current == nil || current.UserID != actor {
				t.Errorf("retained site changed: %+v err=%v", current, err)
			}
		})
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
