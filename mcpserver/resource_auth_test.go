package mcpserver

import (
	"context"
	"testing"
)

func TestMCPPersonalClientLosesActorMembership(t *testing.T) {
	store, site, token := setupMCPStore(t)
	authz, err := store.GetAPIClientAuth(t.Context(), token)
	if err != nil || authz == nil {
		t.Fatalf("initial auth: %+v, %v", authz, err)
	}
	svc := &service{store: store}
	ctx := context.WithValue(t.Context(), authContextKey{}, authz)
	if _, err := svc.resolveSite(ctx, site.ID); err != nil {
		t.Fatal(err)
	}
	teamID, err := store.GetSiteTenantID(t.Context(), site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Exec(t.Context(), "DELETE FROM tenant_members WHERE tenant_id = ? AND user_id = ?", teamID, authz.UserID); err != nil {
		t.Fatal(err)
	}
	authz, err = store.GetAPIClientAuth(t.Context(), token)
	if err != nil || authz == nil {
		t.Fatalf("auth after membership removal: %+v, %v", authz, err)
	}
	ctx = context.WithValue(t.Context(), authContextKey{}, authz)
	if _, err := svc.resolveSite(ctx, site.ID); err == nil {
		t.Fatal("MCP read survived actor membership removal")
	}
	_, result, err := svc.listSites(ctx, nil, struct{}{})
	if err == nil && len(result.Sites) != 0 {
		t.Fatal("MCP discovery exposed sites after membership removal")
	}
}
