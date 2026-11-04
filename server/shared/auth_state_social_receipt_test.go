package shared

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAppleReceiptStateBindingExpiryAndConcurrentConsume(t *testing.T) {
	store := NewAuthStateStore()
	for _, provider := range []string{"google", "github", "microsoft"} {
		state := store.CreateSocialOAuthState(SocialOAuthState{Provider: provider, ExpiresAt: time.Now().Add(time.Minute)})
		if _, ok := store.CreateSocialPOSTReceipt(state, "code", ""); ok {
			t.Fatal("wrong provider receipt")
		}
	}
	expired := store.CreateSocialOAuthState(SocialOAuthState{Provider: "apple", ExpiresAt: time.Now().Add(-time.Minute)})
	if _, ok := store.CreateSocialPOSTReceipt(expired, "code", ""); ok {
		t.Fatal("expired state receipt")
	}
	state := store.CreateSocialOAuthState(SocialOAuthState{Provider: "apple", ExpiresAt: time.Now().Add(time.Minute)})
	receipt, ok := store.CreateSocialPOSTReceipt(state, "code", "")
	if !ok {
		t.Fatal("missing receipt")
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, ok := store.ConsumeSocialPOSTReceipt(receipt); ok {
				winners.Add(1)
			}
		})
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("consume winners: %d", winners.Load())
	}
	state = store.CreateSocialOAuthState(SocialOAuthState{Provider: "apple", ExpiresAt: time.Now().Add(time.Minute)})
	receipt, _ = store.CreateSocialPOSTReceipt(state, "code", "")
	id, _ := uuid.Parse(receipt)
	store.socialPOSTReceipts.Add(id, SocialPOSTReceipt{State: state, ExpiresAt: time.Now().Add(-time.Minute)})
	if _, ok := store.GetSocialPOSTReceipt(receipt); ok {
		t.Fatal("expired receipt available")
	}
}
