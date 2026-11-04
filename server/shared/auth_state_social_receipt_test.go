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

func TestAppleReceiptClearUserPreservesOtherAndAnonymous(t *testing.T) {
	s := NewAuthStateStore()
	user, other := uuid.New(), uuid.New()
	for _, owner := range []uuid.UUID{user, other, uuid.Nil} {
		state := s.CreateSocialOAuthState(SocialOAuthState{UserID: owner, Provider: "apple", ExpiresAt: time.Now().Add(time.Minute)})
		receipt, ok := s.CreateSocialPOSTReceipt(state, "code", "")
		if !ok {
			t.Fatal("create receipt")
		}
		s.ClearUser(user)
		_, receiptAvailable := s.GetSocialPOSTReceipt(receipt)
		_, stateAvailable := s.ConsumeSocialOAuthState(state)
		want := owner != user
		if receiptAvailable != want || stateAvailable != want {
			t.Fatalf("owner=%s receipt=%v state=%v want=%v", owner, receiptAvailable, stateAvailable, want)
		}
	}
}

func TestAppleReceiptClearUserSerializesCreation(t *testing.T) {
	for range 32 {
		s := NewAuthStateStore()
		user := uuid.New()
		state := s.CreateSocialOAuthState(SocialOAuthState{UserID: user, Provider: "apple", ExpiresAt: time.Now().Add(time.Minute)})
		var receipt string
		var wg sync.WaitGroup
		wg.Go(func() { receipt, _ = s.CreateSocialPOSTReceipt(state, "code", "") })
		wg.Go(func() { s.ClearUser(user) })
		wg.Wait()
		if _, ok := s.GetSocialPOSTReceipt(receipt); ok {
			t.Fatal("receipt survived concurrent account removal")
		}
		if _, ok := s.ConsumeSocialOAuthState(state); ok {
			t.Fatal("provider state survived concurrent account removal")
		}
	}
}
