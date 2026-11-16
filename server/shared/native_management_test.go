package shared

import (
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
)

func managementFixture(t *testing.T, s *AuthStateStore) (string, uuid.UUID, string) {
	t.Helper()
	id := uuid.New()
	verifier := "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"
	digest := sha256.Sum256([]byte(verifier))
	handle, err := s.CreateNativeManagement(api.NativeManagementStart{ClientID: "hitkeep-ios", RedirectURI: NativeIOSCallback, CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:]), State: "random-client-state"}, id, "https://example.test/prefix", "security", NativeManagement{Action: "passkey_register"})
	if err != nil {
		t.Fatal(err)
	}
	return handle, id, verifier
}

func TestNativeManagementRequiresPrimaryExactActionAndPersistentSuccess(t *testing.T) {
	s := NewAuthStateStore()
	handle, id, verifier := managementFixture(t, s)
	path := NativeManagementReturnPath(handle)
	csrf, _, _ := s.RenderNativeAuth(handle, "browser", id, "security")
	if _, ok := s.DecideNativeAuth(handle, "browser", csrf, id, "security", true, false); ok {
		t.Fatal("cookie-only approved")
	}
	if s.CompleteAccountDeletionPrimary(path, uuid.New(), "https://example.test/prefix", "primary") {
		t.Fatal("wrong primary user")
	}
	if !s.CompleteAccountDeletionPrimary(path, id, "https://example.test/prefix", "primary") {
		t.Fatal("primary")
	}
	if s.NativeDeletionPrimaryVerified(handle, id, "other-session") {
		t.Fatal("other session")
	}
	code, ok := s.DecideNativeAuth(handle, "browser", csrf, id, "security", true, false)
	if !ok || code != "" {
		t.Fatal("approval must not issue receipt")
	}
	target := NativeManagement{Action: "passkey_register"}
	if s.StartNativeManagementOperation(handle, "other-browser", "primary", id, "security", target) || s.StartNativeManagementOperation(handle, "browser", "other-session", id, "security", target) || s.StartNativeManagementOperation(handle, "browser", "primary", id, "security", NativeManagement{Action: "social_link", Provider: "google"}) {
		t.Fatal("operation binding bypass")
	}
	if !s.StartNativeManagementOperation(handle, "browser", "primary", id, "security", target) {
		t.Fatal("start")
	}
	if !s.BindNativeManagementOperation(handle, "rp-challenge") {
		t.Fatal("challenge binding")
	}
	if s.ClaimNativeManagementMutation(handle, "browser", "primary", id, "security", target, "different-challenge") {
		t.Fatal("wrong challenge")
	}
	var writes atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			if s.ClaimNativeManagementMutation(handle, "browser", "primary", id, "security", target, "rp-challenge") {
				writes.Add(1)
			}
		})
	}
	wg.Wait()
	if writes.Load() != 1 {
		t.Fatalf("writes=%d", writes.Load())
	}
	csrf, _, _ = s.RenderNativeAuth(handle, "browser", id, "security")
	if _, ok := s.DecideNativeAuth(handle, "browser", csrf, id, "security", true, false); ok {
		t.Fatal("receipt without persistent success")
	}
	other, _ := s.CreateNativeManagement(api.NativeManagementStart{}, id, "https://example.test/prefix", "security", target)
	if !s.CompleteNativeManagementMutation(handle, id, "new-security") {
		t.Fatal("persisted success")
	}
	if _, ok := s.GetNativeAuth(other); ok {
		t.Fatal("other attempt survived credential change")
	}
	if _, _, ok := s.RenderNativeAuth(handle, "other-browser", id, "new-security"); ok {
		t.Fatal("receipt moved browser")
	}
	csrf, _, ok = s.RenderNativeAuth(handle, "browser", id, "new-security")
	if !ok {
		t.Fatal("own mutation destroyed receipt")
	}
	code, ok = s.DecideNativeAuth(handle, "browser", csrf, id, "new-security", true, false)
	if !ok || code == "" {
		t.Fatal("receipt")
	}
	input := api.NativeAuthExchange{ClientID: "hitkeep-ios", RedirectURI: NativeIOSCallback, Code: code, CodeVerifier: verifier}
	if _, ok := s.ConsumeNativeAuth(input, "https://example.test/prefix", func(NativeAuthRequest) bool { return true }); ok {
		t.Fatal("management minted login")
	}
	wrong := input
	wrong.CodeVerifier += "x"
	if _, ok := s.ConsumeNativeManagement(wrong, "https://example.test/prefix", func(NativeAuthRequest) bool { return true }); ok {
		t.Fatal("wrong verifier")
	}
	var receipts atomic.Int32
	for range 12 {
		wg.Go(func() {
			if _, ok := s.ConsumeNativeManagement(input, "https://example.test/prefix", func(NativeAuthRequest) bool { return true }); ok {
				receipts.Add(1)
			}
		})
	}
	wg.Wait()
	if receipts.Load() != 1 {
		t.Fatalf("receipts=%d", receipts.Load())
	}
}

func TestNativeManagementCredentialChangeExpiryAndPurposeSeparation(t *testing.T) {
	for _, mode := range []string{"credential-change", "expiry", "restart"} {
		t.Run(mode, func(t *testing.T) {
			s := NewAuthStateStore()
			handle, id, _ := managementFixture(t, s)
			switch mode {
			case "credential-change":
				s.InvalidateNativeAuth(id)
			case "expiry":
				s.nativeMu.Lock()
				entry := s.nativeRequests[nativeHash(handle)]
				entry.ExpiresAt = time.Now().Add(-time.Second)
				s.nativeRequests[nativeHash(handle)] = entry
				s.nativeMu.Unlock()
			case "restart":
				s = NewAuthStateStore()
			}
			if s.CompleteAccountDeletionPrimary(NativeManagementReturnPath(handle), id, "https://example.test/prefix", "primary") {
				t.Fatal("stale attempt accepted primary")
			}
		})
	}
	s := NewAuthStateStore()
	handle, _ := s.CreateNativeAuth(api.NativeAuthStart{}, "issuer")
	id := uuid.New()
	csrf, _, _ := s.RenderNativeAuth(handle, "browser", id, "security")
	code, _ := s.DecideNativeAuth(handle, "browser", csrf, id, "security", true, false)
	if _, ok := s.ConsumeNativeManagement(api.NativeAuthExchange{Code: code}, "issuer", func(NativeAuthRequest) bool { return true }); ok {
		t.Fatal("login code accepted as management")
	}
}
