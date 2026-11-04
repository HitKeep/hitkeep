package oauth

import (
	"context"
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/ory/fosite"

	"hitkeep/auth"
	"hitkeep/database"
)

func (p *Provider) agentToken(w http.ResponseWriter, r *http.Request, form url.Values, authorize func(context.Context, *auth.OAuthGrant, uuid.UUID) error) {
	if !p.agentRegistrationEnabled() {
		oauthError(w, 400, "unsupported_grant_type")
		return
	}
	if authorize == nil {
		oauthError(w, 400, "invalid_grant")
		return
	}
	claimGrant := form.Get("grant_type") == agentClaimGrant
	for key := range form {
		if key != "grant_type" && key != "resource" && key != "scope" && key != "client_id" && !(claimGrant && key == "claim_token") && !(!claimGrant && key == "assertion") {
			oauthError(w, 400, "invalid_request")
			return
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), metadataTimeout)
	defer cancel()
	tx, err := p.store.BeginOAuth(ctx)
	if err != nil {
		oauthError(w, 503, "temporarily_unavailable")
		return
	}
	defer func() { _ = tx.Rollback() }()
	var grantID uuid.UUID
	var pending *database.OAuthPending
	if claimGrant {
		raw := form.Get("claim_token")
		if !validOpaqueToken(raw, agentClaimPrefix) {
			oauthError(w, 400, "expired_token")
			return
		}
		state, record, err := p.loadAgentPending(ctx, tx, tokenSignature(raw), "")
		if err != nil {
			if errors.Is(err, database.ErrOAuthNotFound) || errors.Is(err, ErrInvalidConsent) {
				oauthError(w, 400, "expired_token")
			} else {
				oauthError(w, 503, "temporarily_unavailable")
			}
			return
		}
		pending = record
		now := time.Now().UTC()
		if now.Before(state.NextPoll) {
			state.Interval = min(state.Interval+agentPollInterval, 60)
			state.NextPoll = now.Add(time.Duration(state.Interval) * time.Second)
			record.Payload, err = json.Marshal(state)
			if err == nil {
				err = tx.UpdateAgentPending(ctx, *record)
			}
			if err == nil && (!p.Ready() || !p.agentRegistrationEnabled()) {
				err = ErrUnavailable
			}
			if err == nil {
				err = tx.Commit()
			}
			if err != nil {
				oauthError(w, 503, "temporarily_unavailable")
				return
			}
			oauthError(w, 400, "slow_down")
			return
		}
		if state.Denied {
			if err := tx.ConsumePending(ctx, record.Signature, record.BrowserSignature); err != nil {
				oauthError(w, 503, "temporarily_unavailable")
				return
			}
			if err := tx.Commit(); err != nil {
				oauthError(w, 503, "temporarily_unavailable")
				return
			}
			oauthError(w, 400, "access_denied")
			return
		}
		if state.GrantID == uuid.Nil() {
			if !now.Before(state.CodeExpiresAt) {
				oauthError(w, 400, "expired_token")
				return
			}
			state.NextPoll = now.Add(time.Duration(state.Interval) * time.Second)
			record.Payload, err = json.Marshal(state)
			if err == nil {
				err = tx.UpdateAgentPending(ctx, *record)
			}
			if err == nil && (!p.Ready() || !p.agentRegistrationEnabled()) {
				err = ErrUnavailable
			}
			if err == nil {
				err = tx.Commit()
			}
			if err != nil {
				oauthError(w, 503, "temporarily_unavailable")
				return
			}
			oauthError(w, 400, "authorization_pending")
			return
		}
		if state.GrantID != state.ID {
			oauthError(w, 400, "invalid_grant")
			return
		}
		grantID = state.GrantID
	} else {
		grantID, err = p.parseAgentAssertion(form.Get("assertion"))
		if err != nil {
			oauthError(w, 400, "invalid_grant")
			return
		}
	}
	grant, err := tx.GetGrant(ctx, grantID)
	if err != nil {
		if errors.Is(err, database.ErrOAuthNotFound) {
			oauthError(w, 400, "invalid_grant")
		} else {
			oauthError(w, 503, "temporarily_unavailable")
		}
		return
	}
	if !grant.Active || grant.ClientID != agentClientPrefix+grant.ID.String() {
		oauthError(w, 400, "invalid_grant")
		return
	}
	if (form.Has("resource") && form.Get("resource") != grant.Audience) || (form.Has("client_id") && form.Get("client_id") != grant.ClientID) || (form.Has("scope") && strings.Join(strings.Fields(form.Get("scope")), " ") != strings.Join(grant.Scopes, " ")) {
		oauthError(w, 400, "invalid_grant")
		return
	}
	if _, err := p.agentScopes(grant.Audience, strings.Join(grant.Scopes, " ")); err != nil {
		oauthError(w, 400, "invalid_grant")
		return
	}
	principal := &auth.OAuthGrant{ID: grant.ID, UserID: grant.UserID, TeamID: grant.TeamID, SiteIDs: slices.Clone(grant.SiteIDs), Scopes: slices.Clone(grant.Scopes), Audience: grant.Audience}
	if len(grant.Scopes) == 1 && grant.Scopes[0] == ScopeBillingWrite {
		if err := authorize(ctx, principal, uuid.Nil()); err != nil {
			if errors.Is(err, ErrUnavailable) {
				oauthError(w, 503, "temporarily_unavailable")
				return
			}
			oauthError(w, 400, "invalid_grant")
			return
		}
	}
	for _, siteID := range grant.SiteIDs {
		if err := authorize(ctx, principal, siteID); err != nil {
			if errors.Is(err, ErrUnavailable) {
				oauthError(w, 503, "temporarily_unavailable")
				return
			}
			oauthError(w, 400, "invalid_grant")
			return
		}
	}
	response := map[string]any{}
	if claimGrant {
		assertion, expires, err := p.signAgentAssertion(grant)
		if err != nil {
			oauthError(w, 503, "temporarily_unavailable")
			return
		}
		response["identity_assertion"], response["assertion_expires"] = assertion, expires
	}
	raw, expires, err := p.issueAgentAccess(ctx, tx, grant)
	if err == nil && pending != nil {
		err = tx.ConsumePending(ctx, pending.Signature, pending.BrowserSignature)
	}
	if err == nil {
		err = tx.AppendAudit(ctx, grant.UserID, grant.TeamID, grant.ID, "oauth.agent_token_issued")
	}
	if err == nil && (!p.Ready() || !p.agentRegistrationEnabled()) {
		err = ErrUnavailable
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		oauthError(w, 503, "temporarily_unavailable")
		return
	}
	response["access_token"], response["token_type"], response["expires_in"], response["scope"], response["resource"] = raw, "Bearer", max(0, int(time.Until(expires).Seconds())), strings.Join(grant.Scopes, " "), grant.Audience
	writeJSON(w, http.StatusOK, response)
}

func (p *Provider) issueAgentAccess(ctx context.Context, tx *database.OAuthTransaction, grant *database.OAuthGrant) (string, time.Time, error) {
	client := ClientMetadata{ID: grant.ClientID, Name: grant.ClientName, URI: grant.ClientURI, AuthMethod: "none"}
	expires := minTime(time.Now().UTC().Add(accessLifetime), grant.ExpiresAt)
	request := fosite.NewRequest()
	request.ID = grant.ID.String()
	request.Client = p.fositeClient(client)
	request.RequestedScope, request.GrantedScope = slices.Clone(grant.Scopes), slices.Clone(grant.Scopes)
	request.RequestedAudience, request.GrantedAudience = fosite.Arguments{grant.Audience}, fosite.Arguments{grant.Audience}
	request.Session = &oauthSession{Subject: grant.UserID.String(), GrantID: grant.ID, GrantExpiry: grant.ExpiresAt, Expires: map[fosite.TokenType]time.Time{fosite.AccessToken: expires}}
	raw, signature, err := p.strategy.GenerateAccessToken(ctx, request)
	if err != nil {
		return "", time.Time{}, err
	}
	ctx = context.WithValue(ctx, operationKey{}, &providerOperation{tx: tx, client: client, resource: grant.Audience, agentRegistration: true})
	if err := p.storage.CreateAccessTokenSession(ctx, signature, request); err != nil {
		return "", time.Time{}, err
	}
	return raw, expires, nil
}

func (p *Provider) revokeAgentAccess(ctx context.Context, tx *database.OAuthTransaction, credential *database.OAuthCredential, grant *database.OAuthGrant) error {
	if credential.Kind != "access" || grant.ClientID != agentClientPrefix+grant.ID.String() {
		return ErrInvalidToken
	}
	if err := tx.RevokeCredential(ctx, "access", credential.Signature); err != nil {
		return err
	}
	if err := tx.AppendAudit(ctx, grant.UserID, grant.TeamID, grant.ID, "oauth.agent_token_revoked"); err != nil {
		return err
	}
	if !p.Ready() {
		return ErrUnavailable
	}
	return tx.Commit()
}
