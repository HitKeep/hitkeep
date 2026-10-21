package database

import (
	"context"
	"encoding/json/jsontext"
	"strings"
	"uuid"

	googleuuid "github.com/google/uuid"
)

const OAuthManagedClientPrefix = "hk_client_"

type OAuthClientRecord struct {
	ID       string
	Metadata []byte
	Revision string
	TeamID   uuid.UUID
}

func (s *Store) ListOAuthClients(ctx context.Context, teamID uuid.UUID) ([]OAuthClientRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,metadata,revision FROM oauth_clients WHERE tenant_id=? AND revoked_at IS NULL ORDER BY created_at,id`, teamID.String())
	if err != nil {
		return nil, oauthStoreError(err)
	}
	defer rows.Close()
	result := make([]OAuthClientRecord, 0)
	for rows.Next() {
		var c OAuthClientRecord
		if err := rows.Scan(&c.ID, &c.Metadata, &c.Revision); err != nil {
			return nil, oauthStoreError(err)
		}
		result = append(result, c)
	}
	return result, oauthStoreError(rows.Err())
}

func (s *Store) GetOAuthClient(ctx context.Context, id string) (*OAuthClientRecord, error) {
	var c OAuthClientRecord
	var team string
	err := s.db.QueryRowContext(ctx, `SELECT id,metadata,revision,CAST(tenant_id AS VARCHAR) FROM oauth_clients c WHERE id=? AND revoked_at IS NULL AND NOT EXISTS(SELECT 1 FROM tenant_archives WHERE tenant_id=c.tenant_id)`, id).Scan(&c.ID, &c.Metadata, &c.Revision, &team)
	if err == nil {
		c.TeamID, err = uuid.Parse(team)
	}
	return &c, oauthStoreError(err)
}

// The write pins the registration version until consent commits, conflicting with edits/revocation.
func (o *OAuthTransaction) CheckOAuthClient(ctx context.Context, id, revision string) error {
	if revision == "" {
		return nil
	}
	result, err := o.tx.ExecContext(ctx, `UPDATE oauth_clients SET revision=revision WHERE id=? AND revision=? AND revoked_at IS NULL AND NOT EXISTS(SELECT 1 FROM tenant_archives WHERE tenant_id=oauth_clients.tenant_id)`, id, revision)
	if err != nil {
		return oauthStoreError(err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return oauthStoreError(err)
	}
	if n != 1 {
		return ErrOAuthInactive
	}
	return nil
}

// SaveOAuthClient atomically changes registration and invalidates all previously issued grants.
func (s *Store) SaveOAuthClient(ctx context.Context, actorID, teamID uuid.UUID, c OAuthClientRecord, create, revoke bool) error {
	if !strings.HasPrefix(c.ID, OAuthManagedClientPrefix) || (!revoke && (!jsontext.Value(c.Metadata).IsValid() || len(c.Metadata) > 64<<10)) {
		return ErrOAuthInactive
	}
	op, err := s.BeginOAuth(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = op.Rollback() }()
	// Pin current administration authority so membership changes conflict with this mutation.
	authority, err := op.tx.ExecContext(ctx, `UPDATE tenant_members SET role=role WHERE tenant_id=? AND user_id=? AND role IN ('owner','admin') AND NOT EXISTS(SELECT 1 FROM tenant_archives WHERE tenant_id=?)`, teamID.String(), actorID.String(), teamID.String())
	if err != nil {
		return oauthStoreError(err)
	}
	n, err := authority.RowsAffected()
	if err != nil {
		return oauthStoreError(err)
	}
	if n != 1 {
		return ErrOAuthNotFound
	}
	action := "oauth.client_updated"
	if create {
		var count int
		if err := op.tx.QueryRowContext(ctx, `SELECT count(*) FROM oauth_clients WHERE tenant_id=? AND revoked_at IS NULL`, teamID.String()).Scan(&count); err != nil {
			return oauthStoreError(err)
		}
		if count >= 128 {
			return ErrOAuthCapacity
		}
		// Pin a common team row so concurrent creations cannot bypass the capacity bound.
		if _, err := op.tx.ExecContext(ctx, `UPDATE tenant_members SET role=role WHERE tenant_id=? AND role='owner'`, teamID.String()); err != nil {
			return oauthStoreError(err)
		}
		_, err = op.tx.ExecContext(ctx, `INSERT INTO oauth_clients(id,tenant_id,metadata,revision) VALUES(?,?,?,?)`, c.ID, teamID.String(), string(c.Metadata), c.Revision)
		action = "oauth.client_registered"
	} else {
		query := `UPDATE oauth_clients SET metadata=?,revision=? WHERE id=? AND tenant_id=? AND revoked_at IS NULL`
		args := []any{string(c.Metadata), c.Revision, c.ID, teamID.String()}
		if revoke {
			query = `UPDATE oauth_clients SET revoked_at=CURRENT_TIMESTAMP,revision=? WHERE id=? AND tenant_id=? AND revoked_at IS NULL`
			args = []any{c.Revision, c.ID, teamID.String()}
			action = "oauth.client_revoked"
		}
		result, execErr := op.tx.ExecContext(ctx, query, args...)
		if execErr != nil {
			return oauthStoreError(execErr)
		}
		n, execErr := result.RowsAffected()
		if execErr != nil {
			return oauthStoreError(execErr)
		}
		if n != 1 {
			return ErrOAuthNotFound
		}
		_, err = op.tx.ExecContext(ctx, `UPDATE oauth_grants SET revoked_at=CURRENT_TIMESTAMP WHERE client_id=? AND revoked_at IS NULL`, c.ID)
	}
	if err != nil {
		return oauthStoreError(err)
	}
	if err := appendAuditEntryTx(ctx, op.tx, AuditEntryParams{ActorID: googleuuid.UUID(actorID), TeamID: googleuuid.UUID(teamID), Action: action, TargetType: "oauth_client", TargetID: c.ID, Outcome: "success"}); err != nil {
		return err
	}
	return op.Commit()
}
