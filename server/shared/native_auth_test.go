package shared

import (
	"crypto/sha256"
	"encoding/base64"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	json "hitkeep/jsonapi"
)

func TestNativeAuthBindingConsumptionAndClearUser(t *testing.T) {
	s := NewAuthStateStore()
	verifier := "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"
	digest := sha256.Sum256([]byte(verifier))
	request, err := s.CreateNativeAuth(api.NativeAuthStart{ClientID: "hitkeep-ios", RedirectURI: NativeIOSCallback, CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:]), State: "random-client-state", Intent: "login"}, "https://example.test/prefix")
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if !s.BindNativeAuth(request, id) || s.BindNativeAuth(request, uuid.New()) {
		t.Fatal("subject not pinned")
	}
	csrf, entry, ok := s.RenderNativeAuth(request, "browser", id, "security")
	if !ok || entry.UserID != id {
		t.Fatal("render failed")
	}
	if _, ok := s.DecideNativeAuth(request, "other", csrf, id, "security", true, false); ok {
		t.Fatal("wrong browser approved")
	}
	code, ok := s.DecideNativeAuth(request, "browser", csrf, id, "security", true, true)
	if !ok {
		t.Fatal("approval failed")
	}
	if _, ok := s.DecideNativeAuth(request, "browser", csrf, id, "security", true, true); ok {
		t.Fatal("approved twice")
	}
	input := api.NativeAuthExchange{ClientID: "hitkeep-ios", RedirectURI: NativeIOSCallback, Code: code, CodeVerifier: verifier}
	wrong := input
	wrong.CodeVerifier = verifier + "x"
	if _, ok := s.ConsumeNativeAuth(wrong, "https://example.test/prefix", func(NativeAuthRequest) bool { return true }); ok {
		t.Fatal("wrong verifier accepted")
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if entry, ok := s.ConsumeNativeAuth(input, "https://example.test/prefix", func(NativeAuthRequest) bool { return true }); ok {
				if !entry.RememberMe {
					t.Error("remember choice not frozen")
				}
				winners.Add(1)
			}
		})
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("winners=%d", winners.Load())
	}
	request, _ = s.CreateNativeAuth(api.NativeAuthStart{}, "issuer")
	s.BindNativeAuth(request, id)
	s.ClearUser(id)
	if _, ok := s.GetNativeAuth(request); ok {
		t.Fatal("security cleanup retained request")
	}
}

func TestNativeAuthExpiryDenialRestartAndCurrentAccount(t *testing.T) {
	s := NewAuthStateStore()
	id := uuid.New()
	request, _ := s.CreateNativeAuth(api.NativeAuthStart{}, "issuer")
	csrf, _, _ := s.RenderNativeAuth(request, "browser", id, "version1")
	if _, ok := s.DecideNativeAuth(request, "browser", csrf, id, "version2", true, false); ok {
		t.Fatal("changed account approved")
	}
	if _, ok := s.DecideNativeAuth(request, "browser", csrf, id, "version1", false, false); !ok {
		t.Fatal("denial failed")
	}
	if _, ok := s.GetNativeAuth(request); ok {
		t.Fatal("denied request live")
	}
	if _, ok := NewAuthStateStore().GetNativeAuth(request); ok {
		t.Fatal("restart resurrected state")
	}
	request, _ = s.CreateNativeAuth(api.NativeAuthStart{}, "issuer")
	s.nativeMu.Lock()
	e := s.nativeRequests[nativeHash(request)]
	e.ExpiresAt = time.Now().Add(-time.Second)
	s.nativeRequests[nativeHash(request)] = e
	s.nativeMu.Unlock()
	if _, ok := s.GetNativeAuth(request); ok {
		t.Fatal("expired request live")
	}
}

func TestNativeCanonicalIssuer(t *testing.T) {
	data, err := os.ReadFile("testdata/native_issuer_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Input    string  `json:"input"`
		Expected *string `json:"expected"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, tc := range vectors {
		want := ""
		if tc.Expected != nil {
			want = *tc.Expected
		}
		if got := NativeAuthIssuer(tc.Input); got != want {
			t.Errorf("%q = %q want %q", tc.Input, got, want)
		} else if got != "" && NativeAuthIssuer(got) != got {
			t.Errorf("canonical issuer is not stable: %q", got)
		}
	}
}
