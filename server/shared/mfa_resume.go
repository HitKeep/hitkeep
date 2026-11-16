package shared

import (
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/google/uuid"
)

type mfaResume struct {
	BrowserHash [32]byte
	Factors     []string
	Passkey     *protocol.PublicKeyCredentialRequestOptions
}

// BindMFAResume binds the existing verified-primary challenge to its browser.
func (s *AuthStateStore) BindMFAResume(challengeID uuid.UUID, browser string, factors []string, passkey *protocol.PublicKeyCredentialRequestOptions) bool {
	if s == nil || browser == "" {
		return false
	}
	s.loginChallengesMu.Lock()
	defer s.loginChallengesMu.Unlock()
	challenge, ok := s.loginChallenges.Peek(challengeID)
	if !ok || challenge.Flow != "mfa" || !challenge.HasUserID || isExpired(challenge.ExpiresAt) {
		return false
	}
	for id := range s.mfaResumes {
		if entry, ok := s.loginChallenges.Peek(id); !ok || isExpired(entry.ExpiresAt) {
			delete(s.mfaResumes, id)
		}
	}
	if len(s.mfaResumes) >= authStateCacheSize {
		return false
	}
	s.mfaResumes[challengeID] = mfaResume{BrowserHash: nativeHash(browser), Factors: factors, Passkey: passkey}
	return true
}

func (s *AuthStateStore) MFAResume(challengeID uuid.UUID, browser string) ([]string, *protocol.PublicKeyCredentialRequestOptions, bool) {
	if s == nil || browser == "" {
		return nil, nil, false
	}
	s.loginChallengesMu.Lock()
	defer s.loginChallengesMu.Unlock()
	challenge, ok := s.loginChallenges.Peek(challengeID)
	resume, bound := s.mfaResumes[challengeID]
	if !ok || !bound || challenge.Flow != "mfa" || !challenge.HasUserID || isExpired(challenge.ExpiresAt) || !nativeHashMatches(resume.BrowserHash, browser) {
		return nil, nil, false
	}
	return resume.Factors, resume.Passkey, true
}
