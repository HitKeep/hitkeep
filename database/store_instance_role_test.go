package database

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"hitkeep/auth"
)

func TestInstanceRoleMutationPreservesLastOwner(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		t.Run(map[bool]string{false: "sole owner", true: "mutual demotion"}[concurrent], func(t *testing.T) {
			store := setupTenantStore(t)
			first, err := store.CreateUser(t.Context(), "first-owner@example.test", "unused")
			if err != nil {
				t.Fatal(err)
			}
			owners := []uuid.UUID{first}
			if concurrent {
				second, err := store.CreateUser(t.Context(), "second-owner@example.test", "unused")
				if err != nil {
					t.Fatal(err)
				}
				if err := store.UpdateInstanceRole(t.Context(), second, auth.InstanceOwner, first); err != nil {
					t.Fatal(err)
				}
				owners = append(owners, second)
			}
			start := make(chan struct{})
			results := make([]error, len(owners))
			var wg sync.WaitGroup
			for i, owner := range owners {
				wg.Go(func() {
					<-start
					results[i] = store.UpdateInstanceRole(t.Context(), owner, auth.InstanceUser, owners[(i+1)%len(owners)])
				})
			}
			close(start)
			wg.Wait()
			var denied, remaining int
			for _, err := range results {
				if err != nil {
					if !errors.Is(err, ErrLastInstanceOwner) {
						t.Fatalf("unexpected denial: %v", err)
					}
					denied++
				}
			}
			if err := store.DB().QueryRowContext(t.Context(), "SELECT COUNT(*) FROM instance_roles WHERE role='owner'").Scan(&remaining); err != nil {
				t.Fatal(err)
			}
			if denied != 1 || remaining != 1 {
				t.Fatalf("denied=%d remaining owners=%d", denied, remaining)
			}
		})
	}
}
