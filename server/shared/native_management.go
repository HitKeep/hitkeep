package shared

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/auth"
	"hitkeep/socialauth"
)

const NativeManagementBrowserCookie = "hk_native_management_browser"
const NativeManagementHeader = "X-Hitkeep-Native-Management"

var ErrNativeManagementUnavailable = errors.New("management authorization unavailable")

type NativeManagement struct {
	Action        string
	Provider      string
	TeamID        uuid.UUID
	Phase         string
	OperationHash [32]byte
}

func NativeManagementReturnPath(handle string) string {
	return "/auth/native/manage?request=" + url.QueryEscape(handle)
}

func (c *Context) NativeManagementAuthority(r *http.Request, id uuid.UUID, target NativeManagement) (bool, error) {
	available, err := c.HumanSessionUserAvailable(r.Context(), id)
	if err != nil || !available {
		return false, err
	}
	switch target.Action {
	case "passkey_register":
		return target.Provider == "" && target.TeamID == uuid.Nil, nil
	case "social_link":
		if target.TeamID != uuid.Nil {
			return false, nil
		}
		provider, err := socialauth.ConfigForProvider(c.Config, target.Provider)
		return err == nil && provider.Provider == target.Provider, nil
	case "search_console_connect":
		if target.Provider != "" || target.TeamID == uuid.Nil || strings.TrimSpace(c.Config.GoogleSearchConsoleClientID) == "" || strings.TrimSpace(c.Config.GoogleSearchConsoleClientSecret) == "" {
			return false, nil
		}
		return c.userHasTeamCapability(r.Context(), target.TeamID, id, auth.CapTeamManageIntegrations), nil
	default:
		return false, nil
	}
}

func (s *AuthStateStore) CreateNativeManagement(input api.NativeManagementStart, id uuid.UUID, issuer, security string, target NativeManagement) (string, error) {
	if s == nil || id == uuid.Nil || security == "" {
		return "", errors.New("management subject unavailable")
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	handle, err := s.createNativeAuthLocked(api.NativeAuthStart{ClientID: input.ClientID, RedirectURI: input.RedirectURI, CodeChallenge: input.CodeChallenge, CodeChallengeMethod: input.CodeChallengeMethod, State: input.State, Intent: "login"}, issuer)
	if err != nil {
		return "", err
	}
	entry := s.nativeRequests[nativeHash(handle)]
	entry.Start.Purpose = "management"
	entry.UserID, entry.Security = id, security
	target.Phase = "pending"
	entry.Management = target
	s.nativeRequests[nativeHash(handle)] = entry
	return handle, nil
}

// StartNativeManagementOperation is one winner, before creating provider or RP state.
func (s *AuthStateStore) StartNativeManagementOperation(handle, browser, jti string, id uuid.UUID, security string, target NativeManagement) bool {
	return s.transitionNativeManagement(handle, browser, jti, id, security, target, "authorized", "started", "")
}

// ClaimNativeManagementMutation runs before the persistent write, never after it.
func (s *AuthStateStore) ClaimNativeManagementMutation(handle, browser, jti string, id uuid.UUID, security string, target NativeManagement, operation string) bool {
	return operation != "" && s.transitionNativeManagement(handle, browser, jti, id, security, target, "started", "mutating", operation)
}

func (s *AuthStateStore) transitionNativeManagement(handle, browser, jti string, id uuid.UUID, security string, target NativeManagement, from, to, operation string) bool {
	if s == nil || browser == "" || jti == "" {
		return false
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	entry, ok := s.nativeLocked(handle)
	if !ok || entry.Start.Purpose != "management" || entry.UserID != id || !entry.PrimaryComplete || !nativeHashMatches(entry.PrimaryJTIHash, jti) || !nativeHashMatches(entry.BrowserHash, browser) || entry.Security != security || entry.Management.Action != target.Action || entry.Management.Provider != target.Provider || entry.Management.TeamID != target.TeamID || entry.Management.Phase != from {
		return false
	}
	if operation != "" && !nativeHashMatches(entry.Management.OperationHash, operation) {
		return false
	}
	entry.Management.Phase = to
	s.nativeRequests[nativeHash(handle)] = entry
	return true
}

// BindNativeManagementOperation ties the handler-created challenge/provider state to this attempt.
func (s *AuthStateStore) BindNativeManagementOperation(handle, operation string) bool {
	if s == nil || operation == "" {
		return false
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	entry, ok := s.nativeLocked(handle)
	if !ok || entry.Start.Purpose != "management" || entry.Management.Phase != "started" || entry.Management.OperationHash != [32]byte{} {
		return false
	}
	entry.Management.OperationHash = nativeHash(operation)
	s.nativeRequests[nativeHash(handle)] = entry
	return true
}

// CompleteNativeManagementMutation preserves only this operation across its own credential change.
func (s *AuthStateStore) CompleteNativeManagementMutation(handle string, id uuid.UUID, security string) bool {
	if s == nil || security == "" {
		return false
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	entry, ok := s.nativeLocked(handle)
	if !ok || entry.UserID != id || entry.Start.Purpose != "management" || entry.Management.Phase != "mutating" {
		return false
	}
	entry.Management.Phase = "succeeded"
	entry.Security = security
	entry.CSRFHash = [32]byte{}
	if entry.Management.Action == "passkey_register" || entry.Management.Action == "social_link" {
		s.invalidateNativeAuthLocked(id, nativeHash(handle))
	}
	s.nativeRequests[nativeHash(handle)] = entry
	return true
}

// NativeManagementOperation validates current browser authority; the reference alone carries none.
func (c *Context) NativeManagementOperation(w http.ResponseWriter, r *http.Request, handle string, target NativeManagement, operation string, start bool) bool {
	NativeManagementHeaders(w)
	deny := func() bool { http.Error(w, "Management authorization unavailable", http.StatusForbidden); return false }
	if !c.NativeAuthReady() {
		http.Error(w, "Service not available", http.StatusServiceUnavailable)
		return false
	}
	issuer, _ := url.Parse(NativeAuthIssuer(c.Config.PublicURL))
	// Provider GET callbacks keep their browser-owned OAuth state; operation POSTs require Origin.
	if r.Method == http.MethodPost && (len(r.Header.Values("Origin")) != 1 || r.Header.Get("Origin") != issuer.Scheme+"://"+issuer.Host) {
		return deny()
	}
	id, session, err := c.CurrentHumanSession(r)
	if err != nil {
		http.Error(w, "Service not available", http.StatusServiceUnavailable)
		return false
	}
	if id == uuid.Nil || session == nil {
		return deny()
	}
	allowed, err := c.NativeManagementAuthority(r, id, target)
	if err != nil {
		http.Error(w, "Service not available", http.StatusServiceUnavailable)
		return false
	}
	if !allowed {
		return deny()
	}
	cookie, err := r.Cookie(NativeManagementBrowserCookie)
	if err != nil {
		return deny()
	}
	_, security, err := c.NativeAccountSecurity(r.Context(), id)
	if err != nil {
		http.Error(w, "Service not available", http.StatusServiceUnavailable)
		return false
	}
	if start {
		if !c.AuthState.StartNativeManagementOperation(handle, cookie.Value, session.ID, id, security, target) {
			return deny()
		}
	} else if !c.AuthState.ClaimNativeManagementMutation(handle, cookie.Value, session.ID, id, security, target, operation) {
		return deny()
	}
	return true
}

func NativeManagementHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
}

func NativeManagementReference(r *http.Request) (string, bool) {
	values := r.Header.Values(NativeManagementHeader)
	if len(values) == 0 {
		return "", true
	}
	return r.Header.Get(NativeManagementHeader), len(values) == 1 && validAuthHandle(values[0])
}

func (c *Context) FinishNativeManagementMutation(r *http.Request, handle string, id uuid.UUID) bool {
	_, security, err := c.NativeAccountSecurity(r.Context(), id)
	if err != nil {
		c.AuthState.InvalidateNativeAuth(id)
		return false
	}
	return c.AuthState.CompleteNativeManagementMutation(handle, id, security)
}

// CommitNativeManagementConnection runs under the store lifecycle lock, then nativeMu;
// invalidation cannot pass the final attempt check before persistence and success marking.
func (c *Context) CommitNativeManagementConnection(r *http.Request, handle string, teamID uuid.UUID, operation string, persist func() error) error {
	id, session, err := c.CurrentHumanSession(r)
	if err != nil {
		return err
	}
	cookie, cookieErr := r.Cookie(NativeManagementBrowserCookie)
	if id == uuid.Nil || session == nil || cookieErr != nil || operation == "" {
		return ErrNativeManagementUnavailable
	}
	_, security, err := c.NativeAccountSecurity(r.Context(), id)
	if err != nil {
		return err
	}
	s := c.AuthState
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	entry, ok := s.nativeLocked(handle)
	if !ok || entry.Start.Purpose != "management" || entry.UserID != id || !entry.PrimaryComplete || !nativeHashMatches(entry.PrimaryJTIHash, session.ID) || !nativeHashMatches(entry.BrowserHash, cookie.Value) || entry.Security != security || entry.Management.Action != "search_console_connect" || entry.Management.Provider != "" || entry.Management.TeamID != teamID || entry.Management.Phase != "mutating" || !nativeHashMatches(entry.Management.OperationHash, operation) {
		return ErrNativeManagementUnavailable
	}
	if err := persist(); err != nil {
		return err
	}
	entry.Management.Phase = "succeeded"
	entry.CSRFHash = [32]byte{}
	s.nativeRequests[nativeHash(handle)] = entry
	return nil
}

func (s *AuthStateStore) FailNativeManagement(handle string) {
	if s == nil {
		return
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	entry, ok := s.nativeLocked(handle)
	if ok && entry.Start.Purpose == "management" && entry.Management.Phase != "succeeded" {
		entry.Management.Phase = "failed"
		s.nativeRequests[nativeHash(handle)] = entry
	}
}
