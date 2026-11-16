package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"hitkeep/auth"
)

func TestRegisteredInstanceRoleMutationAuthority(t *testing.T) {
	for _, tc := range []struct {
		name, actor, target string
		role                auth.InstanceRole
		want                int
	}{
		{"admin cannot promote self", "admin", "admin", auth.InstanceOwner, http.StatusForbidden},
		{"admin cannot assign owner", "admin", "user", auth.InstanceOwner, http.StatusForbidden},
		{"admin cannot demote existing owner", "admin", "owner", auth.InstanceUser, http.StatusForbidden},
		{"admin cannot rewrite existing owner", "admin", "owner", auth.InstanceOwner, http.StatusForbidden},
		{"admin cannot change ordinary roles", "admin", "user", auth.InstanceAdmin, http.StatusForbidden},
		{"user cannot assign owner", "user", "user", auth.InstanceOwner, http.StatusForbidden},
		{"owner can assign owner", "owner", "user", auth.InstanceOwner, http.StatusOK},
		{"owner can demote admin to user", "owner", "admin", auth.InstanceUser, http.StatusOK},
		{"owner can promote user to admin", "owner", "user", auth.InstanceAdmin, http.StatusOK},
		{"owner can change another owner", "owner", "other-owner", auth.InstanceAdmin, http.StatusOK},
		{"owner cannot change own role", "owner", "owner", auth.InstanceUser, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, store, _, _, admin, owner := setupAdminTestEnv(t)
			if err := store.UpdateInstanceRole(t.Context(), admin, auth.InstanceAdmin, owner); err != nil {
				t.Fatal(err)
			}
			user, err := store.CreateUser(t.Context(), "ordinary-role-fixture@example.test", "unused")
			if err != nil {
				t.Fatal(err)
			}
			actors := map[string]uuid.UUID{"owner": owner, "admin": admin, "user": user}
			if tc.target == "other-owner" {
				if err := store.UpdateInstanceRole(t.Context(), user, auth.InstanceOwner, owner); err != nil {
					t.Fatal(err)
				}
				actors["other-owner"] = user
			}
			actor, target := actors[tc.actor], actors[tc.target]
			before, err := store.GetInstanceRole(t.Context(), target)
			if err != nil {
				t.Fatal(err)
			}
			token, err := auth.GenerateToken(h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL, actor)
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			Register(mux, h.ctx)
			r := httptest.NewRequest(http.MethodPost, "/api/admin/users/"+target.String()+"/role", strings.NewReader(`{"role":"`+string(tc.role)+`"}`))
			r.Header.Set("Content-Type", "application/json")
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
			out := httptest.NewRecorder()
			mux.ServeHTTP(out, r)
			if out.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", out.Code, tc.want, out.Body.String())
			}
			after, err := store.GetInstanceRole(t.Context(), target)
			if err != nil {
				t.Fatal(err)
			}
			want := before
			if tc.want == http.StatusOK {
				want = tc.role
			}
			if after != want {
				t.Fatalf("persisted role=%q want=%q", after, want)
			}
		})
	}
}

func TestRegisteredInstanceOwnersCannotDemoteEachOtherToZeroOwners(t *testing.T) {
	h, store, _, _, second, first := setupAdminTestEnv(t)
	if err := store.UpdateInstanceRole(t.Context(), second, auth.InstanceOwner, first); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, h.ctx)
	owners := []uuid.UUID{first, second}
	var requests [2]*http.Request
	for i, actor := range owners {
		token, err := auth.GenerateToken(h.ctx.Config.JWTSecret, h.ctx.Config.PublicURL, actor)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, "/api/admin/users/"+owners[1-i].String()+"/role", strings.NewReader(`{"role":"user"}`))
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		requests[i] = r
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var statuses [2]int
	for i := range requests {
		wg.Go(func() {
			<-start
			out := httptest.NewRecorder()
			mux.ServeHTTP(out, requests[i])
			statuses[i] = out.Code
		})
	}
	close(start)
	wg.Wait()
	var remaining int
	if err := store.DB().QueryRowContext(t.Context(), "SELECT COUNT(*) FROM instance_roles WHERE role='owner'").Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatalf("remaining owners=%d statuses=%v", remaining, statuses)
	}
	var changed, denied int
	for _, status := range statuses {
		switch status {
		case http.StatusOK:
			changed++
		case http.StatusForbidden, http.StatusConflict:
			denied++
		default:
			t.Fatalf("unexpected statuses=%v", statuses)
		}
	}
	if changed != 1 || denied != 1 {
		t.Fatalf("statuses=%v", statuses)
	}
	t.Logf("mutual demotion HTTP=%v remaining owners=%d", statuses, remaining)
}
