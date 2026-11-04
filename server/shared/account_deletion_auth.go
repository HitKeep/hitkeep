package shared

import (
	"encoding/hex"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/database"
)

// Browser reauthentication follows the configured instance; native handoff stays HTTPS-only.
func AccountDeletionIssuer(raw string) string { return canonicalAuthIssuer(raw, true) }

func (c *Context) AccountDeletionReady() bool {
	return c != nil && c.Config != nil && AccountDeletionIssuer(c.Config.PublicURL) != "" && c.AuthState != nil && c.Store != nil && c.Store.DatabaseStatus().State == database.DatabaseStateHealthy && (c.Cluster == nil || c.Cluster.IsLeader())
}

type accountDeletionAttempt struct {
	UserID                          uuid.UUID
	Issuer, ConsequenceHash         string
	ExpiresAt                       time.Time
	NativeRequestHash, ProofJTIHash [32]byte
	Verified                        bool
}

func AccountDeletionReturnPath(attempt string) string {
	return "/settings?deletion_attempt=" + url.QueryEscape(attempt)
}

func AccountDeletionHandle(returnPath string) string {
	const prefix = "/settings?deletion_attempt="
	handle, ok := strings.CutPrefix(returnPath, prefix)
	if !ok || len(handle) != 43 || AccountDeletionReturnPath(handle) != returnPath {
		return ""
	}
	if !validAuthHandle(handle) {
		return ""
	}
	return handle
}

func validAuthHandle(handle string) bool {
	for _, ch := range handle {
		if !(ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-') {
			return false
		}
	}
	return len(handle) == 43
}

func (s *AuthStateStore) CreateAccountDeletionAttempt(userID uuid.UUID, issuer, consequenceHash, returnPath string, expiresAt time.Time) string {
	issuer = AccountDeletionIssuer(issuer)
	now := time.Now().UTC()
	digest, err := hex.DecodeString(consequenceHash)
	if s == nil || userID == uuid.Nil || issuer == "" || returnPath != "/settings" || err != nil || len(digest) != 32 || strings.ToLower(consequenceHash) != consequenceHash || !expiresAt.After(now) || expiresAt.After(now.Add(NativeAuthTTL)) {
		return ""
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	for key, entry := range s.accountDeletionAttempts {
		if isExpired(entry.ExpiresAt) {
			delete(s.accountDeletionAttempts, key)
		}
	}
	if len(s.accountDeletionAttempts) >= authStateCacheSize {
		return ""
	}
	handle := NativeAuthToken()
	s.accountDeletionAttempts[nativeHash(handle)] = accountDeletionAttempt{UserID: userID, Issuer: issuer, ConsequenceHash: consequenceHash, ExpiresAt: expiresAt.UTC()}
	return handle
}

func (s *AuthStateStore) deletionLocked(attemptID string) (accountDeletionAttempt, bool) {
	entry, ok := s.accountDeletionAttempts[nativeHash(attemptID)]
	return entry, ok && validAuthHandle(attemptID) && !isExpired(entry.ExpiresAt)
}

// AccountDeletionSubject returns only the frozen subject of an active attempt.
func (s *AuthStateStore) AccountDeletionSubject(returnPath, issuer string) (uuid.UUID, bool) {
	issuer = AccountDeletionIssuer(issuer)
	if s == nil {
		return uuid.Nil, false
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	if handle := NativeAuthHandle(returnPath); handle != "" {
		native, ok := s.nativeLocked(handle)
		if !ok || native.Start.Purpose != "account_delete" || native.Issuer != issuer {
			return uuid.Nil, false
		}
		entry, ok := s.deletionLocked(native.Start.ReauthAttempt)
		return entry.UserID, ok && entry.Issuer == issuer && entry.NativeRequestHash == nativeHash(handle)
	}
	entry, ok := s.deletionLocked(AccountDeletionHandle(returnPath))
	return entry.UserID, ok && entry.Issuer == issuer
}

// CompleteAccountDeletionPrimary is called only after actual primary plus MFA.
// A native primary records readiness only; its browser session gets no proof.
func (s *AuthStateStore) CompleteAccountDeletionPrimary(returnPath string, userID uuid.UUID, issuer, jti string) bool {
	issuer = AccountDeletionIssuer(issuer)
	if s == nil || jti == "" || userID == uuid.Nil {
		return false
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	if handle := NativeAuthHandle(returnPath); handle != "" {
		native, ok := s.nativeLocked(handle)
		if !ok || native.Issuer != issuer || native.UserID != userID {
			return false
		}
		if native.Start.Purpose == "" {
			return true
		}
		entry, ok := s.deletionLocked(native.Start.ReauthAttempt)
		if !ok || entry.UserID != userID || entry.Issuer != issuer || entry.NativeRequestHash != nativeHash(handle) || entry.Verified {
			return false
		}
		native.PrimaryJTIHash = nativeHash(jti)
		native.PrimaryComplete = true
		s.nativeRequests[nativeHash(handle)] = native
		return true
	}
	handle := AccountDeletionHandle(returnPath)
	entry, ok := s.deletionLocked(handle)
	if !ok || entry.UserID != userID || entry.Issuer != issuer || entry.NativeRequestHash != [32]byte{} || entry.Verified {
		return false
	}
	entry.ProofJTIHash = nativeHash(jti)
	entry.Verified = true
	s.accountDeletionAttempts[nativeHash(handle)] = entry
	return true
}

func (s *AuthStateStore) NativeDeletionPrimaryVerified(handle string, userID uuid.UUID, jti string) bool {
	if s == nil {
		return false
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	native, ok := s.nativeLocked(handle)
	if !ok {
		return false
	}
	if native.Start.Purpose == "" {
		return true
	}
	entry, ok := s.deletionLocked(native.Start.ReauthAttempt)
	return ok && entry.NativeRequestHash == nativeHash(handle) && entry.UserID == userID && entry.Issuer == native.Issuer && native.UserID == userID && native.PrimaryComplete && jti != "" && nativeHashMatches(native.PrimaryJTIHash, jti)
}

func (s *AuthStateStore) BindNativeAccountDeletionProof(native NativeAuthRequest, userID uuid.UUID, jti string) bool {
	if s == nil {
		return false
	}
	if native.Start.Purpose == "" {
		return true
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	entry, ok := s.deletionLocked(native.Start.ReauthAttempt)
	if !ok || !native.ExchangeConsumed || !native.Approved || !native.PrimaryComplete || entry.Verified || entry.UserID != userID || native.UserID != userID || entry.Issuer != native.Issuer || entry.NativeRequestHash != native.RequestHash || jti == "" || nativeHashMatches(native.PrimaryJTIHash, jti) {
		return false
	}
	entry.Verified = true
	entry.ProofJTIHash = nativeHash(jti)
	s.accountDeletionAttempts[nativeHash(native.Start.ReauthAttempt)] = entry
	return true
}

func (s *AuthStateStore) AccountDeletionProofVerified(attemptID string, userID uuid.UUID, currentFreshJTI, consequenceHash string) bool {
	if s == nil {
		return false
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	return s.deletionProofLocked(attemptID, userID, currentFreshJTI, consequenceHash)
}
func (s *AuthStateStore) deletionProofLocked(attemptID string, userID uuid.UUID, jti, consequenceHash string) bool {
	entry, ok := s.deletionLocked(attemptID)
	return ok && entry.Verified && entry.UserID == userID && userID != uuid.Nil && jti != "" && nativeHashMatches(entry.ProofJTIHash, jti) && entry.ConsequenceHash == consequenceHash
}
func (s *AuthStateStore) ConsumeAccountDeletionProof(attemptID string, userID uuid.UUID, currentFreshJTI, consequenceHash string) bool {
	if s == nil {
		return false
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	if !s.deletionProofLocked(attemptID, userID, currentFreshJTI, consequenceHash) {
		return false
	}
	delete(s.accountDeletionAttempts, nativeHash(attemptID))
	return true
}
