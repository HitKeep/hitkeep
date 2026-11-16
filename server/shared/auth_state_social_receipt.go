package shared

import (
	"time"

	"github.com/google/uuid"
)

// SocialPOSTReceipt bridges an external POST to a same-origin GET with the original Lax cookie.
type SocialPOSTReceipt struct {
	State     string
	Code      string
	Error     string
	ExpiresAt time.Time
}

func (s *AuthStateStore) CreateSocialPOSTReceipt(state, code, providerError string) (string, bool) {
	if s == nil {
		return "", false
	}
	id, err := uuid.Parse(state)
	if err != nil || id == uuid.Nil {
		return "", false
	}
	s.socialOAuthMu.Lock()
	defer s.socialOAuthMu.Unlock()
	flow, ok := s.socialOAuth.Get(id)
	if !ok || flow.Provider != "apple" || isExpired(flow.ExpiresAt) || flow.AppleReceiptID != "" {
		return "", false
	}
	receiptID := uuid.New()
	flow.AppleReceiptID = receiptID.String()
	s.socialOAuth.Add(id, flow)
	s.socialPOSTMu.Lock()
	defer s.socialPOSTMu.Unlock()
	s.socialPOSTReceipts.Add(receiptID, SocialPOSTReceipt{State: state, Code: code, Error: providerError, ExpiresAt: time.Now().UTC().Add(90 * time.Second)})
	return receiptID.String(), true
}

func (s *AuthStateStore) GetSocialPOSTReceipt(receipt string) (SocialPOSTReceipt, bool) {
	if s == nil {
		return SocialPOSTReceipt{}, false
	}
	id, err := uuid.Parse(receipt)
	if err != nil || id == uuid.Nil {
		return SocialPOSTReceipt{}, false
	}
	s.socialPOSTMu.Lock()
	defer s.socialPOSTMu.Unlock()
	entry, ok := s.socialPOSTReceipts.Get(id)
	return entry, ok && !isExpired(entry.ExpiresAt)
}

func (s *AuthStateStore) ConsumeSocialPOSTReceipt(receipt string) (SocialPOSTReceipt, bool) {
	if s == nil {
		return SocialPOSTReceipt{}, false
	}
	id, err := uuid.Parse(receipt)
	if err != nil || id == uuid.Nil {
		return SocialPOSTReceipt{}, false
	}
	s.socialPOSTMu.Lock()
	defer s.socialPOSTMu.Unlock()
	entry, ok := s.socialPOSTReceipts.Get(id)
	s.socialPOSTReceipts.Remove(id)
	return entry, ok && !isExpired(entry.ExpiresAt)
}
