package oauth

import (
	"context"
	"encoding/json/v2"
	"errors"
	"maps"
	"net/url"
	"slices"
	"time"
	"uuid"

	"github.com/ory/fosite"

	"hitkeep/database"
)

type operationKey struct{}
type providerOperation struct {
	tx                 *database.OAuthTransaction
	client             ClientMetadata
	resource           string
	securityRevocation bool
	rollback           bool
	revoking           bool
}

func operationFrom(ctx context.Context) (*providerOperation, error) {
	op, ok := ctx.Value(operationKey{}).(*providerOperation)
	if !ok || op == nil || op.tx == nil {
		return nil, fosite.ErrServerError
	}
	return op, nil
}

type oauthSession struct {
	Subject     string
	GrantID     uuid.UUID
	GrantExpiry time.Time
	Expires     map[fosite.TokenType]time.Time
}

func (s *oauthSession) SetExpiresAt(kind fosite.TokenType, expires time.Time) {
	if s.Expires == nil {
		s.Expires = make(map[fosite.TokenType]time.Time)
	}
	if !s.GrantExpiry.IsZero() && s.GrantExpiry.Before(expires) {
		expires = s.GrantExpiry
	}
	s.Expires[kind] = expires
}
func (s *oauthSession) GetExpiresAt(kind fosite.TokenType) time.Time { return s.Expires[kind] }
func (s *oauthSession) GetUsername() string                          { return "" }
func (s *oauthSession) GetSubject() string                           { return s.Subject }
func (s *oauthSession) Clone() fosite.Session {
	clone := *s
	clone.Expires = maps.Clone(s.Expires)
	return &clone
}

type storedRequest struct {
	ID              string         `json:"id"`
	RequestedAt     time.Time      `json:"requested_at"`
	Client          ClientMetadata `json:"client"`
	RequestedScopes []string       `json:"requested_scopes"`
	GrantedScopes   []string       `json:"granted_scopes"`
	Audience        string         `json:"audience"`
	RedirectURI     string         `json:"redirect_uri,omitempty"`
	Challenge       string         `json:"code_challenge,omitempty"`
	ChallengeMethod string         `json:"code_challenge_method,omitempty"`
	Session         oauthSession   `json:"session"`
}

type fositeStorage struct{ provider *Provider }

func (s *fositeStorage) GetClient(ctx context.Context, id string) (fosite.Client, error) {
	op, err := operationFrom(ctx)
	if err != nil || op.client.ID != id {
		return nil, fosite.ErrNotFound
	}
	return s.provider.fositeClient(op.client), nil
}
func (*fositeStorage) ClientAssertionJWTValid(context.Context, string) error {
	return fosite.ErrInvalidClient
}
func (*fositeStorage) SetClientAssertionJWT(context.Context, string, time.Time) error {
	return fosite.ErrInvalidClient
}
func (*fositeStorage) BeginTX(ctx context.Context) (context.Context, error) {
	_, err := operationFrom(ctx)
	return ctx, err
}

// Fosite's nested transaction hooks never publish before the outer operation finishes.
func (*fositeStorage) Commit(ctx context.Context) error { _, err := operationFrom(ctx); return err }
func (*fositeStorage) Rollback(ctx context.Context) error {
	op, err := operationFrom(ctx)
	if err == nil {
		op.rollback = true
	}
	return err
}

func storageError(err error) error {
	if errors.Is(err, database.ErrOAuthNotFound) {
		return fosite.ErrNotFound
	}
	if errors.Is(err, database.ErrOAuthConsumed) {
		return fosite.ErrInactiveToken
	}
	if errors.Is(err, database.ErrOAuthConflict) {
		return fosite.ErrSerializationFailure
	}
	return err
}
func (s *fositeStorage) save(ctx context.Context, kind, signature, accessSignature string, request fosite.Requester) error {
	op, err := operationFrom(ctx)
	if err != nil {
		return err
	}
	session, ok := request.GetSession().(*oauthSession)
	if !ok || session == nil {
		return fosite.ErrServerError
	}
	if request.GetID() != session.GrantID.String() || op.client.ID != request.GetClient().GetID() {
		return fosite.ErrServerError
	}
	snapshot := storedRequest{ID: request.GetID(), RequestedAt: request.GetRequestedAt(), Client: cloneClientMetadata(op.client), RequestedScopes: slices.Clone(request.GetRequestedScopes()), GrantedScopes: slices.Clone(request.GetGrantedScopes()), Audience: op.resource, Session: *session}
	snapshot.RedirectURI = request.GetRequestForm().Get("redirect_uri")
	snapshot.Challenge = request.GetRequestForm().Get("code_challenge")
	snapshot.ChallengeMethod = request.GetRequestForm().Get("code_challenge_method")
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	tokenKind := fosite.AuthorizeCode
	if kind == "access" {
		tokenKind = fosite.AccessToken
	} else if kind == "refresh" {
		tokenKind = fosite.RefreshToken
	}
	grant, err := op.tx.GetGrant(ctx, session.GrantID)
	if err != nil {
		return storageError(err)
	}
	return storageError(op.tx.CreateCredential(ctx, database.OAuthCredential{Kind: kind, Signature: signature, GrantID: session.GrantID, TeamID: grant.TeamID, Payload: payload, AccessSignature: accessSignature, ExpiresAt: session.GetExpiresAt(tokenKind)}))
}
func (s *fositeStorage) load(ctx context.Context, kind, signature string) (fosite.Requester, error) {
	op, err := operationFrom(ctx)
	if err != nil {
		return nil, err
	}
	credential, err := op.tx.GetCredential(ctx, kind, signature)
	if err != nil {
		return nil, storageError(err)
	}
	grant, err := op.tx.GetGrant(ctx, credential.GrantID)
	if err != nil {
		return nil, storageError(err)
	}
	var snapshot storedRequest
	if err := json.Unmarshal(credential.Payload, &snapshot); err != nil {
		return nil, fosite.ErrServerError
	}
	if snapshot.ID != grant.ID.String() || snapshot.Session.GrantID != grant.ID || snapshot.Client.ID != grant.ClientID || snapshot.Audience != grant.Audience || op.client.ID != grant.ClientID || op.resource != grant.Audience {
		return nil, fosite.ErrNotFound
	}
	request := fosite.NewRequest()
	request.ID = snapshot.ID
	request.RequestedAt = snapshot.RequestedAt
	request.Client = s.provider.fositeClient(snapshot.Client)
	request.RequestedScope = snapshot.RequestedScopes
	request.GrantedScope = snapshot.GrantedScopes
	request.RequestedAudience = fosite.Arguments{snapshot.Audience}
	request.GrantedAudience = fosite.Arguments{snapshot.Audience}
	request.Session = &snapshot.Session
	request.Form = url.Values{"redirect_uri": {snapshot.RedirectURI}, "code_challenge": {snapshot.Challenge}, "code_challenge_method": {snapshot.ChallengeMethod}}
	if op.revoking {
		return request, nil
	}
	if credential.Consumed {
		if kind == "code" {
			return request, fosite.ErrInvalidatedAuthorizeCode
		}
		return request, fosite.ErrInactiveToken
	}
	if credential.Revoked || !grant.Active {
		return request, fosite.ErrNotFound
	}
	return request, nil
}
func (s *fositeStorage) CreateAuthorizeCodeSession(ctx context.Context, sig string, r fosite.Requester) error {
	return s.save(ctx, "code", sig, "", r)
}
func (s *fositeStorage) GetAuthorizeCodeSession(ctx context.Context, sig string, session fosite.Session) (fosite.Requester, error) {
	request, err := s.load(ctx, "code", sig)
	if request != nil {
		if target, ok := session.(*oauthSession); ok && target != nil && target.GrantID == uuid.Nil() {
			*target = *request.GetSession().Clone().(*oauthSession)
		}
	}
	return request, err
}
func (s *fositeStorage) InvalidateAuthorizeCodeSession(ctx context.Context, sig string) error {
	op, err := operationFrom(ctx)
	if err != nil {
		return err
	}
	return storageError(op.tx.ConsumeCredential(ctx, "code", sig))
}
func (s *fositeStorage) CreateAccessTokenSession(ctx context.Context, sig string, r fosite.Requester) error {
	return s.save(ctx, "access", sig, "", r)
}
func (s *fositeStorage) GetAccessTokenSession(ctx context.Context, sig string, _ fosite.Session) (fosite.Requester, error) {
	return s.load(ctx, "access", sig)
}
func (s *fositeStorage) DeleteAccessTokenSession(ctx context.Context, sig string) error {
	op, err := operationFrom(ctx)
	if err != nil {
		return err
	}
	return storageError(op.tx.RevokeCredential(ctx, "access", sig))
}
func (s *fositeStorage) CreateRefreshTokenSession(ctx context.Context, sig, accessSig string, r fosite.Requester) error {
	return s.save(ctx, "refresh", sig, accessSig, r)
}
func (s *fositeStorage) GetRefreshTokenSession(ctx context.Context, sig string, _ fosite.Session) (fosite.Requester, error) {
	return s.load(ctx, "refresh", sig)
}
func (s *fositeStorage) DeleteRefreshTokenSession(ctx context.Context, sig string) error {
	op, err := operationFrom(ctx)
	if err != nil {
		return err
	}
	err = op.tx.ConsumeCredential(ctx, "refresh", sig)
	if errors.Is(err, database.ErrOAuthConsumed) {
		return nil
	}
	return storageError(err)
}
func (s *fositeStorage) RotateRefreshToken(ctx context.Context, requestID, sig string) error {
	op, err := operationFrom(ctx)
	if err != nil {
		return err
	}
	c, err := op.tx.GetCredential(ctx, "refresh", sig)
	if err != nil {
		return storageError(err)
	}
	if c.GrantID.String() != requestID {
		return fosite.ErrNotFound
	}
	if err := op.tx.ConsumeCredential(ctx, "refresh", sig); err != nil {
		return storageError(err)
	}
	if c.AccessSignature != "" {
		return storageError(op.tx.RevokeCredential(ctx, "access", c.AccessSignature))
	}
	return nil
}
func (s *fositeStorage) revokeFamily(ctx context.Context, requestID string) error {
	op, err := operationFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(requestID)
	if err != nil {
		return fosite.ErrNotFound
	}
	if err := op.tx.RevokeGrant(ctx, id); err != nil {
		return storageError(err)
	}
	op.securityRevocation = true
	return nil
}
func (s *fositeStorage) RevokeRefreshToken(ctx context.Context, id string) error {
	return s.revokeFamily(ctx, id)
}
func (s *fositeStorage) RevokeAccessToken(ctx context.Context, id string) error {
	return s.revokeFamily(ctx, id)
}
func (s *fositeStorage) CreatePKCERequestSession(ctx context.Context, sig string, r fosite.Requester) error {
	return s.save(ctx, "pkce", sig, "", r)
}
func (s *fositeStorage) GetPKCERequestSession(ctx context.Context, sig string, _ fosite.Session) (fosite.Requester, error) {
	return s.load(ctx, "pkce", sig)
}
func (s *fositeStorage) DeletePKCERequestSession(ctx context.Context, sig string) error {
	op, err := operationFrom(ctx)
	if err != nil {
		return err
	}
	return storageError(op.tx.ConsumeCredential(ctx, "pkce", sig))
}
