package database

import (
	"context"
	"errors"
	"time"
)

// GetAgentPending accepts one hashed claim handle or one hashed browser-attempt handle.
func (o *OAuthTransaction) GetAgentPending(ctx context.Context, claim, attempt string) (*OAuthPending, error) {
	if (claim == "") == (attempt == "") || (claim != "" && len(claim) != 64) || (attempt != "" && len(attempt) != 64) {
		return nil, ErrOAuthNotFound
	}
	var pending OAuthPending
	var payload string
	err := o.tx.QueryRowContext(ctx, `SELECT signature,browser_signature,payload,expires_at FROM oauth_pending WHERE (signature=? OR browser_signature=?) AND expires_at>?`, claim, attempt, time.Now().UTC()).Scan(&pending.Signature, &pending.BrowserSignature, &payload, &pending.ExpiresAt)
	if err != nil {
		return nil, oauthStoreError(err)
	}
	pending.Payload = []byte(payload)
	return &pending, nil
}

// UpdateAgentPending cannot extend the original pending request's lifetime.
func (o *OAuthTransaction) UpdateAgentPending(ctx context.Context, pending OAuthPending) error {
	if len(pending.Signature) != 64 || len(pending.BrowserSignature) != 64 || len(pending.Payload) > 64<<10 || pending.ExpiresAt.IsZero() {
		return errors.New("invalid OAuth pending request")
	}
	var signature string
	err := o.tx.QueryRowContext(ctx, `UPDATE oauth_pending SET browser_signature=?,payload=? WHERE signature=? AND expires_at=? AND expires_at>? RETURNING signature`, pending.BrowserSignature, string(pending.Payload), pending.Signature, pending.ExpiresAt, time.Now().UTC()).Scan(&signature)
	return oauthStoreError(err)
}
