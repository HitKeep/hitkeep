package database

import "testing"

func TestAccountDeletionSSOLookupRequiresExistingSubjectWithoutEmailLink(t *testing.T) {
	s := setupTenantStore(t)
	ctx := t.Context()
	owner, err := s.CreateUser(ctx, "existing-sso@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.GetActiveTenantID(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if user, err := s.GetUserBySSOIdentity(ctx, team, "https://issuer.example.test", "unlinked-subject"); err != nil || user != nil {
		t.Fatalf("unlinked subject=%v %v", user, err)
	}
	linked, err := s.ResolveSSOUser(ctx, ResolveSSOUserInput{TeamID: team, IssuerURL: "https://issuer.example.test", Subject: "linked-subject", Email: "existing-sso@example.test", PasswordHash: "unused"})
	if err != nil || linked.UserID != owner {
		t.Fatalf("seed identity=%+v %v", linked, err)
	}
	if user, err := s.GetUserBySSOIdentity(ctx, team, "https://issuer.example.test", "linked-subject"); err != nil || user == nil || user.ID != owner {
		t.Fatalf("linked subject=%v %v", user, err)
	}
	if user, err := s.GetUserBySSOIdentity(ctx, team, "https://other-issuer.example.test", "linked-subject"); err != nil || user != nil {
		t.Fatalf("wrong issuer=%v %v", user, err)
	}
	if user, err := s.GetUserBySSOIdentity(ctx, team, "https://issuer.example.test", "different-subject"); err != nil || user != nil {
		t.Fatalf("wrong subject=%v %v", user, err)
	}
}
