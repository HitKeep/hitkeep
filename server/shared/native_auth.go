package shared

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"hitkeep/api"
)

const (
	NativeIOSCallback     = "https://hitkeep.com/auth/native/ios/callback"
	NativeAndroidCallback = "https://hitkeep.com/auth/native/android/callback"
	NativeAuthTTL         = 10 * time.Minute
)

// NativeAuthRequest freezes all client bindings; only explicit consent can approve it.
// Lookup handles, browser nonces, CSRF tokens and exchange codes are stored as hashes.
type NativeAuthRequest struct {
	Management NativeManagement

	PrimaryJTIHash   [32]byte
	PrimaryComplete  bool
	ExchangeConsumed bool
	RequestHash      [32]byte
	Start            api.NativeAuthStart
	Issuer           string
	ExpiresAt        time.Time
	UserID           uuid.UUID
	Security         string
	BrowserHash      [32]byte
	CSRFHash         [32]byte
	CodeHash         [32]byte
	Approved         bool
	RememberMe       bool
}

func nativeHashMatches(expected [32]byte, value string) bool {
	actual := nativeHash(value)
	return subtle.ConstantTimeCompare(expected[:], actual[:]) == 1
}

func nativeHash(value string) [32]byte { return sha256.Sum256([]byte(value)) }

func NativeAuthToken() string {
	var value [32]byte
	_, _ = rand.Read(value[:])
	return base64.RawURLEncoding.EncodeToString(value[:])
}

func NativeAuthIssuer(raw string) string { return canonicalAuthIssuer(raw, false) }

func canonicalAuthIssuer(raw string, allowHTTP bool) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && (!allowHTTP || u.Scheme != "http")) || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" || u.EscapedPath() != u.Path || strings.ContainsAny(raw, "#\\\r\n\t") || strings.ContainsFunc(raw, unicode.IsSpace) {
		return ""
	}
	if port := u.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return ""
		}
	}
	prefix := strings.TrimSuffix(u.Path, "/")
	if strings.Contains(u.Path, "//") || (prefix != "" && path.Clean(prefix) != prefix) {
		return ""
	}
	u.Host = strings.ToLower(u.Host)
	if (u.Scheme == "https" && u.Port() == "443") || (u.Scheme == "http" && u.Port() == "80") {
		u.Host = strings.ToLower(u.Hostname())
		if strings.Contains(u.Host, ":") {
			u.Host = "[" + u.Host + "]"
		}
	}
	u.Path = prefix
	return u.String()
}

func NativeAuthClient(client, callback string) bool {
	return (client == "hitkeep-ios" && callback == NativeIOSCallback) || (client == "hitkeep-android" && callback == NativeAndroidCallback)
}

func NativeAuthReturnPath(handle string) string {
	return "/auth/native/consent?request=" + url.QueryEscape(handle)
}

func NativeAuthHandle(returnPath string) string {
	if len(returnPath) > 256 {
		return ""
	}
	u, err := url.ParseRequestURI(returnPath)
	if err != nil || (u.Path != "/auth/native/consent" && u.Path != "/auth/native/manage") || u.IsAbs() || u.Host != "" || u.Fragment != "" {
		return ""
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q) != 1 || len(q["request"]) != 1 {
		return ""
	}
	handle := q.Get("request")
	if len(handle) != 43 {
		return ""
	}
	if _, err := base64.RawURLEncoding.DecodeString(handle); err != nil {
		return ""
	}
	return handle
}

func (c *Context) NativeAuthReady() bool {
	return c.AccountDeletionReady() && NativeAuthIssuer(c.Config.PublicURL) != ""
}

func (c *Context) NativeAuthStatus() *api.NativeAuthStatus {
	if c == nil || c.Config == nil {
		return nil
	}
	issuer := NativeAuthIssuer(c.Config.PublicURL)
	if issuer == "" {
		return nil
	}
	available := c.NativeAuthReady()
	androidOrigins, androidErr := c.Config.AndroidPasskeyOrigins()
	return &api.NativeAuthStatus{Enabled: true, Available: available, Issuer: issuer, StartURL: issuer + "/api/auth/native/start", AuthorizeURL: issuer + "/api/auth/native/authorize", ExchangeURL: issuer + "/api/auth/native/exchange", CodeChallengeMethodsSupported: []string{"S256"}, BrowserPasskeys: true,
		// Associated-domain delivery is a release prerequisite; arbitrary self-hosts use browser passkeys.
		NativeIOSPasskeys: c.Config.CloudHosted && (issuer == "https://cloud.hitkeep.eu" || issuer == "https://cloud.hitkeep.com"), NativeAndroidPasskeys: available && androidErr == nil && len(androidOrigins) > 0, Management: &api.NativeManagementStatus{StartURL: issuer + "/api/auth/native/management/start", CompleteURL: issuer + "/api/auth/native/management/complete", ActionsSupported: []string{"passkey_register", "social_link", "search_console_connect"}}}
}

func (s *AuthStateStore) pruneNativeLocked() {
	for key, entry := range s.nativeRequests {
		if isExpired(entry.ExpiresAt) {
			delete(s.nativeCodes, entry.CodeHash)
			delete(s.nativeRequests, key)
		}
	}
}

func (s *AuthStateStore) CreateNativeAuth(start api.NativeAuthStart, issuer string) (string, error) {
	if s == nil {
		return "", errors.New("native auth unavailable")
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	return s.createNativeAuthLocked(start, issuer)
}

func (s *AuthStateStore) createNativeAuthLocked(start api.NativeAuthStart, issuer string) (string, error) {
	s.pruneNativeLocked()
	if len(s.nativeRequests) >= authStateCacheSize {
		return "", errors.New("native auth capacity reached")
	}
	handle := NativeAuthToken()
	entry := NativeAuthRequest{Start: start, Issuer: issuer, ExpiresAt: time.Now().UTC().Add(NativeAuthTTL), RequestHash: nativeHash(handle)}
	if start.Purpose != "" || start.ReauthAttempt != "" {
		attempt, ok := s.deletionLocked(start.ReauthAttempt)
		if start.Purpose != "account_delete" || start.Intent != "login" || !ok || attempt.Issuer != issuer || attempt.Verified {
			return "", errors.New("invalid deletion attempt")
		}
		// A new native ceremony replaces only the delivery binding, never the consequences.
		if prior, ok := s.nativeRequests[attempt.NativeRequestHash]; ok {
			delete(s.nativeCodes, prior.CodeHash)
			delete(s.nativeRequests, attempt.NativeRequestHash)
		}
		attempt.NativeRequestHash = nativeHash(handle)
		s.accountDeletionAttempts[nativeHash(start.ReauthAttempt)] = attempt
		entry.UserID = attempt.UserID
		entry.ExpiresAt = minTime(entry.ExpiresAt, attempt.ExpiresAt)
	}
	s.nativeRequests[nativeHash(handle)] = entry
	return handle, nil
}

func (s *AuthStateStore) GetNativeAuth(handle string) (NativeAuthRequest, bool) {
	if s == nil {
		return NativeAuthRequest{}, false
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	return s.nativeLocked(handle)
}

func (s *AuthStateStore) nativeLocked(handle string) (NativeAuthRequest, bool) {
	entry, ok := s.nativeRequests[nativeHash(handle)]
	return entry, ok && !isExpired(entry.ExpiresAt) && !entry.Approved
}

func (s *AuthStateStore) BindNativeAuth(handle string, userID uuid.UUID) bool {
	if s == nil || userID == uuid.Nil {
		return false
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	entry, ok := s.nativeLocked(handle)
	if !ok || (entry.UserID != uuid.Nil && entry.UserID != userID) {
		return false
	}
	entry.UserID = userID
	s.nativeRequests[nativeHash(handle)] = entry
	return true
}

// BindNativeReturnPath is called only after primary authentication proves the subject.
func (s *AuthStateStore) BindNativeReturnPath(returnPath string, userID uuid.UUID) bool {
	handle := NativeAuthHandle(returnPath)
	return handle == "" || s.BindNativeAuth(handle, userID)
}

func (s *AuthStateStore) RenderNativeAuth(handle, browser string, userID uuid.UUID, security string) (string, NativeAuthRequest, bool) {
	if s == nil || browser == "" || userID == uuid.Nil {
		return "", NativeAuthRequest{}, false
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	entry, ok := s.nativeLocked(handle)
	if !ok || (entry.UserID != uuid.Nil && entry.UserID != userID) {
		return "", NativeAuthRequest{}, false
	}
	csrf := NativeAuthToken()
	entry.UserID = userID
	if entry.Start.Purpose == "management" && (entry.Security != security || entry.Management.Phase != "pending" && !nativeHashMatches(entry.BrowserHash, browser)) {
		return "", NativeAuthRequest{}, false
	}
	entry.Security = security
	entry.BrowserHash = nativeHash(browser)
	entry.CSRFHash = nativeHash(csrf)
	s.nativeRequests[nativeHash(handle)] = entry
	return csrf, entry, true
}

func (s *AuthStateStore) DecideNativeAuth(handle, browser, csrf string, userID uuid.UUID, security string, approve, remember bool) (string, bool) {
	if s == nil || browser == "" || csrf == "" {
		return "", false
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	entry, ok := s.nativeLocked(handle)
	if !ok || entry.UserID != userID || entry.Security != security || !nativeHashMatches(entry.BrowserHash, browser) || !nativeHashMatches(entry.CSRFHash, csrf) {
		return "", false
	}
	if approve && entry.Start.Purpose != "" && !entry.PrimaryComplete {
		return "", false
	}
	if !approve {
		delete(s.nativeRequests, nativeHash(handle))
		return "", true
	}
	if entry.Start.Purpose == "management" {
		if remember {
			return "", false
		}
		switch entry.Management.Phase {
		case "pending":
			entry.Management.Phase = "authorized"
			entry.CSRFHash = [32]byte{}
			s.nativeRequests[nativeHash(handle)] = entry
			return "", true
		case "succeeded":
		default:
			return "", false
		}
	}
	code := NativeAuthToken()
	entry.Approved = true
	entry.RememberMe = remember
	entry.CodeHash = nativeHash(code)
	entry.ExpiresAt = minTime(entry.ExpiresAt, time.Now().UTC().Add(time.Minute))
	s.nativeRequests[nativeHash(handle)] = entry
	s.nativeCodes[entry.CodeHash] = nativeHash(handle)
	return code, true
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// ConsumeNativeAuth holds the transition lock through current-account validation;
// only the winner may issue cookies. Issuance failure must never restore a code.
func (s *AuthStateStore) ConsumeNativeAuth(input api.NativeAuthExchange, issuer string, current func(NativeAuthRequest) bool) (NativeAuthRequest, bool) {
	return s.consumeNativeAuth(input, issuer, false, current)
}

func (s *AuthStateStore) ConsumeNativeManagement(input api.NativeAuthExchange, issuer string, current func(NativeAuthRequest) bool) (NativeAuthRequest, bool) {
	return s.consumeNativeAuth(input, issuer, true, current)
}

func (s *AuthStateStore) consumeNativeAuth(input api.NativeAuthExchange, issuer string, management bool, current func(NativeAuthRequest) bool) (NativeAuthRequest, bool) {
	if s == nil {
		return NativeAuthRequest{}, false
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	key, ok := s.nativeCodes[nativeHash(input.Code)]
	if !ok {
		return NativeAuthRequest{}, false
	}
	entry, ok := s.nativeRequests[key]
	digest := sha256.Sum256([]byte(input.CodeVerifier))
	if !ok || (entry.Start.Purpose == "management") != management || !entry.Approved || isExpired(entry.ExpiresAt) || entry.Issuer != issuer || entry.Start.ClientID != input.ClientID || entry.Start.RedirectURI != input.RedirectURI || entry.Start.CodeChallenge != base64.RawURLEncoding.EncodeToString(digest[:]) || !current(entry) {
		return NativeAuthRequest{}, false
	}
	delete(s.nativeCodes, entry.CodeHash)
	delete(s.nativeRequests, key)
	entry.ExchangeConsumed = true
	return entry, true
}

// Credential changes and logout invalidate both native handoffs and deletion proof.
func (s *AuthStateStore) InvalidateNativeAuth(userID uuid.UUID) {
	if s == nil || userID == uuid.Nil {
		return
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	s.invalidateNativeAuthLocked(userID, [32]byte{})
}

func (s *AuthStateStore) invalidateNativeAuthLocked(userID uuid.UUID, except [32]byte) {
	for key, entry := range s.accountDeletionAttempts {
		if entry.UserID == userID {
			delete(s.accountDeletionAttempts, key)
		}
	}
	for key, entry := range s.nativeRequests {
		if entry.UserID == userID && key != except {
			delete(s.nativeCodes, entry.CodeHash)
			delete(s.nativeRequests, key)
		}
	}
}

// SaveNativeEmailReturn binds a server-created one-time email token to its native
// continuation. A different email browser resumes the same bounded transaction.
func (s *AuthStateStore) SaveNativeEmailReturn(token, returnPath string) {
	handle := NativeAuthHandle(returnPath)
	if s == nil || handle == "" || token == "" {
		return
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	if _, ok := s.nativeLocked(handle); !ok {
		return
	}
	for key, link := range s.nativeEmailReturns {
		if isExpired(link.ExpiresAt) {
			delete(s.nativeEmailReturns, key)
		}
	}
	if len(s.nativeEmailReturns) >= authStateCacheSize {
		return
	}
	s.nativeEmailReturns[nativeHash(token)] = nativeEmailReturn{ReturnPath: returnPath, ExpiresAt: time.Now().UTC().Add(NativeAuthTTL)}
}

type nativeEmailReturn struct {
	ReturnPath string
	ExpiresAt  time.Time
}

func (s *AuthStateStore) NativeEmailReturn(token string) string {
	if s == nil {
		return ""
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	entry, ok := s.nativeEmailReturns[nativeHash(token)]
	if !ok || isExpired(entry.ExpiresAt) {
		return ""
	}
	return entry.ReturnPath
}
