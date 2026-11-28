//go:build billing

package ingest

import (
	"hitkeep/config"
	"hitkeep/database"
	"hitkeep/server/shared"
	"hitkeep/testutil/testdb"
	"testing"
	"time"
)

func TestMonthlyQuotaAcrossSitesAndPaidUpgrade(t *testing.T) {
	store := testdb.Shared(t)
	owner, err := store.CreateManagedCloudAccount(t.Context(), database.CreateManagedCloudAccountInput{Email: "meter@example.test", HashedPassword: "hash", TeamName: "Usage"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CreateSite(t.Context(), owner.UserID, "first.example")
	if err != nil {
		t.Fatal(err)
	}
	next, err := store.CreateSite(t.Context(), owner.UserID, "second.example")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Exec(t.Context(), `INSERT INTO site_activity_hourly_counts(site_id,tenant_id,bucket,hits,events) VALUES(?,?,?,?,?)`, first.ID, owner.TenantID, time.Now().UTC().Truncate(time.Hour), 10000, 1999); err != nil {
		t.Fatal(err)
	}
	h := &handler{ctx: &shared.Context{Store: store, Config: &config.Config{CloudHosted: true}}}
	if h.dropMonthlyHit(t.Context(), first.ID) || !h.dropMonthlyHit(t.Context(), next.ID) {
		t.Fatal("team grace allowance must be shared across sites")
	}
	if err := store.UpsertCloudBillingAccount(t.Context(), database.CloudBillingAccount{TenantID: owner.TenantID, PlanCode: database.CloudPlanPro, PlanName: "Pro", SubscriptionStatus: "active"}); err != nil {
		t.Fatal(err)
	}
	if h.dropMonthlyHit(t.Context(), next.ID) {
		t.Fatal("upgrade must lift cached free cap immediately")
	}
}
