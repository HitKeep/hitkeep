package database

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAgentPendingRotationCannotExtendWindowOrReuseOldAttempt(t *testing.T) {
	store, _, _ := setupAPIClientStore(t)
	t.Cleanup(func() { _ = store.Close() })
	pending := OAuthPending{Signature: strings.Repeat("a", 64), BrowserSignature: strings.Repeat("b", 64), Payload: []byte(`{"type":"service_auth"}`), ExpiresAt: time.Now().UTC().Add(10 * time.Minute)}
	if err := store.CreateOAuthPending(t.Context(), pending); err != nil {
		t.Fatal(err)
	}
	tx, err := store.BeginOAuth(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	loaded, err := tx.GetAgentPending(t.Context(), pending.Signature, "")
	if err != nil {
		t.Fatal(err)
	}
	loaded.ExpiresAt = loaded.ExpiresAt.Add(time.Hour)
	if err := tx.UpdateAgentPending(t.Context(), *loaded); !errors.Is(err, ErrOAuthNotFound) {
		t.Fatalf("extended original claim window: %v", err)
	}
	loaded, err = tx.GetAgentPending(t.Context(), pending.Signature, "")
	if err != nil {
		t.Fatal(err)
	}
	loaded.BrowserSignature = strings.Repeat("c", 64)
	if err := tx.UpdateAgentPending(t.Context(), *loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.GetAgentPending(t.Context(), "", pending.BrowserSignature); !errors.Is(err, ErrOAuthNotFound) {
		t.Fatalf("old attempt survived rotation: %v", err)
	}
	if _, err := tx.GetAgentPending(t.Context(), "", loaded.BrowserSignature); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
