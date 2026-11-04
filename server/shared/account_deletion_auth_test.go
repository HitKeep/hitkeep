package shared

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"hitkeep/api"
)

func TestAccountDeletionProofBindingsAndOneConsumer(t *testing.T) {
	s := NewAuthStateStore()
	id := uuid.New()
	hash := strings.Repeat("a", 64)
	attempt := s.CreateAccountDeletionAttempt(id, "https://example.test/prefix", hash, "/settings", time.Now().Add(time.Minute))
	if len(attempt) != 43 {
		t.Fatal("missing opaque attempt")
	}
	path := AccountDeletionReturnPath(attempt)
	if s.AccountDeletionProofVerified(attempt, id, "current", hash) || s.ConsumeAccountDeletionProof(attempt, id, "current", hash) {
		t.Fatal("ambient session accepted")
	}
	if s.CompleteAccountDeletionPrimary(path, uuid.New(), "https://example.test/prefix", "fresh") || s.CompleteAccountDeletionPrimary(path, id, "https://other.test", "fresh") || s.CompleteAccountDeletionPrimary(path, id, "https://example.test/prefix", "") {
		t.Fatal("wrong binding accepted")
	}
	if !s.CompleteAccountDeletionPrimary(path, id, "https://example.test/prefix", "fresh") {
		t.Fatal("verified primary refused")
	}
	for _, args := range []struct {
		id        uuid.UUID
		jti, hash string
	}{{uuid.New(), "fresh", hash}, {id, "current", hash}, {id, "fresh", strings.Repeat("b", 64)}, {id, "", hash}} {
		if s.AccountDeletionProofVerified(attempt, args.id, args.jti, args.hash) || s.ConsumeAccountDeletionProof(attempt, args.id, args.jti, args.hash) {
			t.Fatal("wrong destructive binding accepted")
		}
	}
	if !s.AccountDeletionProofVerified(attempt, id, "fresh", hash) || !s.AccountDeletionProofVerified(attempt, id, "fresh", hash) {
		t.Fatal("read mutated proof")
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			if s.ConsumeAccountDeletionProof(attempt, id, "fresh", hash) {
				winners.Add(1)
			}
		})
	}
	wg.Wait()
	if winners.Load() != 1 || s.CompleteAccountDeletionPrimary(path, id, "https://example.test/prefix", "new") {
		t.Fatal("replay")
	}
}

func TestAccountDeletionAttemptValidationExpiryAndClear(t *testing.T) {
	s := NewAuthStateStore()
	id := uuid.New()
	hash := strings.Repeat("a", 64)
	for _, tc := range []struct {
		issuer, hash, path string
		expiry             time.Time
	}{{"https://example.test", hash, "/settings", time.Time{}}, {"https://example.test", hash, "/settings", time.Now().Add(11 * time.Minute)}, {"https://example.test", strings.Repeat("A", 64), "/settings", time.Now().Add(time.Minute)}, {"https://example.test", hash, "/else", time.Now().Add(time.Minute)}, {"https://example.test/?x=1", hash, "/settings", time.Now().Add(time.Minute)}} {
		if s.CreateAccountDeletionAttempt(id, tc.issuer, tc.hash, tc.path, tc.expiry) != "" {
			t.Fatal("invalid attempt")
		}
	}
	attempt := s.CreateAccountDeletionAttempt(id, "https://example.test", hash, "/settings", time.Now().Add(time.Minute))
	s.ClearUser(id)
	if s.CompleteAccountDeletionPrimary(AccountDeletionReturnPath(attempt), id, "https://example.test", "fresh") {
		t.Fatal("cleared attempt survived")
	}
	attempt = s.CreateAccountDeletionAttempt(id, "https://example.test", hash, "/settings", time.Now().Add(time.Minute))
	s.nativeMu.Lock()
	entry := s.accountDeletionAttempts[nativeHash(attempt)]
	entry.ExpiresAt = time.Now().Add(-time.Second)
	s.accountDeletionAttempts[nativeHash(attempt)] = entry
	s.nativeMu.Unlock()
	if s.CompleteAccountDeletionPrimary(AccountDeletionReturnPath(attempt), id, "https://example.test", "fresh") {
		t.Fatal("expired attempt")
	}
}

func TestNativeDeletionProofOnlyExchangeSession(t *testing.T) {
	s := NewAuthStateStore()
	id := uuid.New()
	hash := strings.Repeat("a", 64)
	issuer := "https://example.test"
	attempt := s.CreateAccountDeletionAttempt(id, issuer, hash, "/settings", time.Now().Add(time.Minute))
	digest := sha256.Sum256([]byte("verifier"))
	start := api.NativeAuthStart{ClientID: "hitkeep-ios", RedirectURI: NativeIOSCallback, CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:]), Purpose: "account_delete", ReauthAttempt: attempt, Intent: "login"}
	handle, err := s.CreateNativeAuth(start, issuer)
	if err != nil {
		t.Fatal(err)
	}
	if s.CompleteAccountDeletionPrimary(AccountDeletionReturnPath(attempt), id, issuer, "browser") {
		t.Fatal("native attempt marked by direct browser")
	}
	path := NativeAuthReturnPath(handle)
	if s.NativeDeletionPrimaryVerified(handle, id, "browser") {
		t.Fatal("cookie proved primary")
	}
	if !s.CompleteAccountDeletionPrimary(path, id, issuer, "browser") || !s.NativeDeletionPrimaryVerified(handle, id, "browser") {
		t.Fatal("primary not retained")
	}
	if s.AccountDeletionProofVerified(attempt, id, "browser", hash) {
		t.Fatal("browser cookie gained native proof")
	}
	csrf, _, ok := s.RenderNativeAuth(handle, "browser-nonce", id, "security")
	if !ok {
		t.Fatal("render")
	}
	code, ok := s.DecideNativeAuth(handle, "browser-nonce", csrf, id, "security", true, false)
	if !ok {
		t.Fatal("approve")
	}
	entry, ok := s.ConsumeNativeAuth(api.NativeAuthExchange{ClientID: start.ClientID, RedirectURI: start.RedirectURI, Code: code, CodeVerifier: "verifier"}, issuer, func(NativeAuthRequest) bool { return true })
	if !ok {
		t.Fatal("exchange")
	}
	if !s.BindNativeAccountDeletionProof(entry, id, "native") {
		t.Fatal("exchange proof failed")
	}
	if s.AccountDeletionProofVerified(attempt, id, "browser", hash) || !s.ConsumeAccountDeletionProof(attempt, id, "native", hash) {
		t.Fatal("native JTI binding failed")
	}
}
