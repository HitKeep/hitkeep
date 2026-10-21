package database

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"time"
	"uuid"

	duckdb "github.com/duckdb/duckdb-go/v2"
	googleuuid "github.com/google/uuid"
)

var (
	ErrOAuthNotFound = errors.New("OAuth record not found")
	ErrOAuthConsumed = errors.New("OAuth credential already consumed")
	ErrOAuthInactive = errors.New("OAuth grant inactive")
	ErrOAuthConflict = errors.New("OAuth operation conflicted")
	ErrOAuthCapacity = errors.New("OAuth capacity exhausted")
)

type OAuthGrant struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	TeamID         uuid.UUID
	ClientRevision string
	ClientID       string
	ClientName     string
	ClientURI      string
	Audience       string
	Scopes         []string
	SiteIDs        []uuid.UUID
	CreatedAt      time.Time
	ExpiresAt      time.Time
	LastUsedAt     *time.Time
	RevokedAt      *time.Time
	Active         bool
}

type OAuthCredential struct {
	Kind            string
	Signature       string
	GrantID         uuid.UUID
	TeamID          uuid.UUID
	Payload         []byte
	AccessSignature string
	ExpiresAt       time.Time
	Consumed        bool
	Revoked         bool
}

type OAuthPending struct {
	Signature        string
	BrowserSignature string
	Payload          []byte
	ExpiresAt        time.Time
}

// OAuthTransaction is one protocol operation, including its validation reads.
type OAuthTransaction struct{ tx *sql.Tx }

func (s *Store) BeginOAuth(ctx context.Context) (*OAuthTransaction, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, oauthStoreError(err)
	}
	return &OAuthTransaction{tx: tx}, nil
}
func (o *OAuthTransaction) Commit() error { return oauthStoreError(o.tx.Commit()) }
func (o *OAuthTransaction) Rollback() error {
	err := o.tx.Rollback()
	if errors.Is(err, sql.ErrTxDone) {
		return nil
	}
	return oauthStoreError(err)
}

func oauthStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrOAuthNotFound
	}
	if e, ok := errors.AsType[*duckdb.Error](err); ok && strings.Contains(strings.ToLower(e.Msg), "conflict") {
		return fmt.Errorf("%w: concurrent control-plane write", ErrOAuthConflict)
	}
	return fmt.Errorf("OAuth persistence: %w", err)
}

func (o *OAuthTransaction) CreateGrant(ctx context.Context, g OAuthGrant) error {
	if err := o.pruneExpiredGrants(ctx); err != nil {
		return err
	}
	if g.ID == uuid.Nil() || g.UserID == uuid.Nil() || g.TeamID == uuid.Nil() || len(g.SiteIDs) == 0 || len(g.SiteIDs) > 512 || len(g.Scopes) == 0 || len(g.Scopes) > 4 || g.ClientID == "" || g.ClientName == "" || g.Audience == "" || !g.ExpiresAt.After(g.CreatedAt) {
		return errors.New("invalid OAuth grant")
	}
	scopes, err := json.Marshal(g.Scopes)
	if err != nil {
		return err
	}
	var count int
	if err := o.tx.QueryRowContext(ctx, "SELECT count(*) FROM oauth_grants WHERE user_id=? AND revoked_at IS NULL AND expires_at>?", g.UserID.String(), time.Now().UTC()).Scan(&count); err != nil {
		return oauthStoreError(err)
	}
	if count >= 200 {
		return ErrOAuthCapacity
	}
	result, err := o.tx.ExecContext(ctx, `INSERT INTO oauth_grants(id,user_id,tenant_id,client_id,client_name,client_uri,audience,scopes,created_at,expires_at,client_registration_revision)
 SELECT ?,?,?,?,?,?,?,?,?,?,? WHERE EXISTS(SELECT 1 FROM tenant_members WHERE tenant_id=? AND user_id=?) AND NOT EXISTS(SELECT 1 FROM tenant_archives WHERE tenant_id=?) AND (?='' OR EXISTS(SELECT 1 FROM oauth_clients WHERE id=? AND tenant_id=? AND revision=? AND revoked_at IS NULL))`, g.ID.String(), g.UserID.String(), g.TeamID.String(), g.ClientID, g.ClientName, g.ClientURI, g.Audience, string(scopes), g.CreatedAt, g.ExpiresAt, g.ClientRevision, g.TeamID.String(), g.UserID.String(), g.TeamID.String(), g.ClientRevision, g.ClientID, g.TeamID.String(), g.ClientRevision)
	if err != nil {
		return oauthStoreError(err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrOAuthInactive
	}
	seen := make(map[uuid.UUID]bool, len(g.SiteIDs))
	for _, id := range g.SiteIDs {
		if id == uuid.Nil() || seen[id] {
			return errors.New("invalid OAuth grant site")
		}
		seen[id] = true
		result, err := o.tx.ExecContext(ctx, `INSERT INTO oauth_grant_sites(oauth_grant_id,tenant_id,site_id) SELECT ?,?,site_id FROM site_tenants WHERE site_id=? AND tenant_id=?`, g.ID.String(), g.TeamID.String(), id.String(), g.TeamID.String())
		if err != nil {
			return oauthStoreError(err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrOAuthInactive
		}
	}
	return nil
}

func (o *OAuthTransaction) GetGrant(ctx context.Context, id uuid.UUID) (*OAuthGrant, error) {
	var g OAuthGrant
	var idRaw, userRaw, teamRaw, scopes string
	var lastUsed, revoked sql.NullTime
	err := o.tx.QueryRowContext(ctx, `SELECT CAST(g.id AS VARCHAR),CAST(g.user_id AS VARCHAR),CAST(g.tenant_id AS VARCHAR),g.client_id,g.client_name,g.client_uri,g.audience,g.scopes,g.created_at,g.expires_at,g.last_used_at,g.revoked_at,
 EXISTS(SELECT 1 FROM users u JOIN tenant_members tm ON tm.user_id=u.id WHERE u.id=g.user_id AND tm.tenant_id=g.tenant_id) AND NOT EXISTS(SELECT 1 FROM tenant_archives ta WHERE ta.tenant_id=g.tenant_id)
 AND (g.client_registration_revision='' OR EXISTS(SELECT 1 FROM oauth_clients c WHERE c.id=g.client_id AND c.tenant_id=g.tenant_id AND c.revoked_at IS NULL AND NOT EXISTS(SELECT 1 FROM tenant_archives ta WHERE ta.tenant_id=c.tenant_id)))
 FROM oauth_grants g WHERE g.id=?`, id.String()).Scan(&idRaw, &userRaw, &teamRaw, &g.ClientID, &g.ClientName, &g.ClientURI, &g.Audience, &scopes, &g.CreatedAt, &g.ExpiresAt, &lastUsed, &revoked, &g.Active)
	if err != nil {
		return nil, oauthStoreError(err)
	}
	if g.ID, err = uuid.Parse(idRaw); err != nil {
		return nil, err
	}
	if g.UserID, err = uuid.Parse(userRaw); err != nil {
		return nil, err
	}
	if g.TeamID, err = uuid.Parse(teamRaw); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(scopes), &g.Scopes); err != nil {
		return nil, err
	}
	if lastUsed.Valid {
		g.LastUsedAt = new(lastUsed.Time)
	}
	if revoked.Valid {
		g.RevokedAt = new(revoked.Time)
	}
	rows, err := o.tx.QueryContext(ctx, `SELECT CAST(gs.site_id AS VARCHAR) FROM oauth_grant_sites gs JOIN site_tenants st ON st.site_id=gs.site_id AND st.tenant_id=gs.tenant_id WHERE gs.oauth_grant_id=? AND gs.tenant_id=? ORDER BY gs.site_id`, id.String(), g.TeamID.String())
	if err != nil {
		return nil, oauthStoreError(err)
	}
	defer rows.Close()
	g.SiteIDs = make([]uuid.UUID, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, err
		}
		g.SiteIDs = append(g.SiteIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, oauthStoreError(err)
	}
	g.Active = g.Active && g.RevokedAt == nil && time.Now().UTC().Before(g.ExpiresAt) && len(g.SiteIDs) > 0
	return &g, nil
}

func (o *OAuthTransaction) CreateCredential(ctx context.Context, c OAuthCredential) error {
	if c.Kind != "code" && c.Kind != "access" && c.Kind != "refresh" && c.Kind != "pkce" {
		return errors.New("invalid OAuth credential kind")
	}
	if len(c.Signature) != 64 || len(c.Payload) > 64<<10 || c.GrantID == uuid.Nil() || c.TeamID == uuid.Nil() {
		return errors.New("invalid OAuth credential")
	}
	result, err := o.tx.ExecContext(ctx, `INSERT INTO oauth_credentials(kind,signature,oauth_grant_id,tenant_id,payload,access_signature,expires_at) SELECT ?,?,?,?,?,?,? WHERE EXISTS(SELECT 1 FROM oauth_grants WHERE id=? AND tenant_id=? AND revoked_at IS NULL AND expires_at>?)`, c.Kind, c.Signature, c.GrantID.String(), c.TeamID.String(), string(c.Payload), c.AccessSignature, c.ExpiresAt, c.GrantID.String(), c.TeamID.String(), time.Now().UTC())
	if err != nil {
		return oauthStoreError(err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrOAuthInactive
	}
	return nil
}

func (o *OAuthTransaction) GetCredential(ctx context.Context, kind, signature string) (*OAuthCredential, error) {
	var c OAuthCredential
	var grantRaw, teamRaw, payload string
	err := o.tx.QueryRowContext(ctx, `SELECT c.kind,c.signature,CAST(c.oauth_grant_id AS VARCHAR),CAST(c.tenant_id AS VARCHAR),c.payload,c.access_signature,c.expires_at,c.consumed_at IS NOT NULL,c.revoked_at IS NOT NULL FROM oauth_credentials c JOIN oauth_grants g ON g.id=c.oauth_grant_id AND g.tenant_id=c.tenant_id WHERE c.kind=? AND c.signature=?`, kind, signature).Scan(&c.Kind, &c.Signature, &grantRaw, &teamRaw, &payload, &c.AccessSignature, &c.ExpiresAt, &c.Consumed, &c.Revoked)
	if err != nil {
		return nil, oauthStoreError(err)
	}
	var parseErr error
	if c.GrantID, parseErr = uuid.Parse(grantRaw); parseErr != nil {
		return nil, parseErr
	}
	if c.TeamID, parseErr = uuid.Parse(teamRaw); parseErr != nil {
		return nil, parseErr
	}
	c.Payload = []byte(payload)
	return &c, nil
}

func (o *OAuthTransaction) ConsumeCredential(ctx context.Context, kind, signature string) error {
	var consumed string
	err := o.tx.QueryRowContext(ctx, `UPDATE oauth_credentials SET consumed_at=? WHERE kind=? AND signature=? AND consumed_at IS NULL AND revoked_at IS NULL RETURNING signature`, time.Now().UTC(), kind, signature).Scan(&consumed)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrOAuthConsumed
	}
	return oauthStoreError(err)
}

func (o *OAuthTransaction) RevokeCredential(ctx context.Context, kind, signature string) error {
	_, err := o.tx.ExecContext(ctx, `UPDATE oauth_credentials SET revoked_at=COALESCE(revoked_at,?) WHERE kind=? AND signature=?`, time.Now().UTC(), kind, signature)
	return oauthStoreError(err)
}
func (o *OAuthTransaction) RevokeGrant(ctx context.Context, id uuid.UUID) error {
	_, err := o.tx.ExecContext(ctx, `UPDATE oauth_grants SET revoked_at=COALESCE(revoked_at,?) WHERE id=?`, time.Now().UTC(), id.String())
	return oauthStoreError(err)
}
func (o *OAuthTransaction) TouchGrant(ctx context.Context, id uuid.UUID) error {
	_, err := o.tx.ExecContext(ctx, `UPDATE oauth_grants SET last_used_at=? WHERE id=? AND revoked_at IS NULL`, time.Now().UTC(), id.String())
	return oauthStoreError(err)
}

func (s *Store) GetOAuthGrant(ctx context.Context, id uuid.UUID) (*OAuthGrant, error) {
	op, err := s.BeginOAuth(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = op.Rollback() }()
	return op.GetGrant(ctx, id)
}
func (s *Store) RevokeOAuthGrant(ctx context.Context, userID, id uuid.UUID) error {
	op, err := s.BeginOAuth(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = op.Rollback() }()
	var found string
	err = op.tx.QueryRowContext(ctx, `UPDATE oauth_grants SET revoked_at=COALESCE(revoked_at,?) WHERE id=? AND user_id=? RETURNING CAST(id AS VARCHAR)`, time.Now().UTC(), id.String(), userID.String()).Scan(&found)
	if err != nil {
		return oauthStoreError(err)
	}
	grant, err := op.GetGrant(ctx, id)
	if err != nil {
		return err
	}
	if err := op.AppendAudit(ctx, userID, grant.TeamID, id, "oauth.grant_revoked"); err != nil {
		return err
	}
	return op.Commit()
}
func (s *Store) ListOAuthGrants(ctx context.Context, userID uuid.UUID) ([]OAuthGrant, error) {
	op, err := s.BeginOAuth(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = op.Rollback() }()
	rows, err := op.tx.QueryContext(ctx, `SELECT CAST(id AS VARCHAR) FROM oauth_grants WHERE user_id=? AND revoked_at IS NULL AND expires_at>? ORDER BY created_at DESC LIMIT 200`, userID.String(), time.Now().UTC())
	if err != nil {
		return nil, oauthStoreError(err)
	}
	defer func() { _ = rows.Close() }()
	var ids []uuid.UUID
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, err
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rowsErr := rows.Err()
	closeErr := rows.Close()
	if rowsErr != nil {
		return nil, rowsErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	grants := make([]OAuthGrant, 0, len(ids))
	for _, id := range ids {
		g, err := op.GetGrant(ctx, id)
		if err != nil {
			return nil, err
		}
		grants = append(grants, *g)
	}
	return grants, nil
}

func (s *Store) CreateOAuthPending(ctx context.Context, p OAuthPending) error {
	if len(p.Signature) != 64 || len(p.BrowserSignature) != 64 || len(p.Payload) > 64<<10 || !p.ExpiresAt.After(time.Now()) || p.ExpiresAt.After(time.Now().Add(16*time.Minute)) {
		return errors.New("invalid OAuth pending request")
	}
	op, err := s.BeginOAuth(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = op.Rollback() }()
	if _, err := op.tx.ExecContext(ctx, "DELETE FROM oauth_pending WHERE expires_at<=?", time.Now().UTC()); err != nil {
		return oauthStoreError(err)
	}
	var count int
	if err := op.tx.QueryRowContext(ctx, "SELECT count(*) FROM oauth_pending").Scan(&count); err != nil {
		return oauthStoreError(err)
	}
	if count >= 4096 {
		return ErrOAuthCapacity
	}
	if _, err := op.tx.ExecContext(ctx, "DELETE FROM oauth_pending WHERE browser_signature=?", p.BrowserSignature); err != nil {
		return oauthStoreError(err)
	}
	if _, err := op.tx.ExecContext(ctx, `INSERT INTO oauth_pending(signature,browser_signature,payload,expires_at) VALUES(?,?,?,?)`, p.Signature, p.BrowserSignature, string(p.Payload), p.ExpiresAt); err != nil {
		return oauthStoreError(err)
	}
	return op.Commit()
}
func (s *Store) GetOAuthPending(ctx context.Context, signature, browserSignature string) (*OAuthPending, error) {
	var p OAuthPending
	var payload string
	err := s.db.QueryRowContext(ctx, `SELECT signature,browser_signature,payload,expires_at FROM oauth_pending WHERE signature=? AND browser_signature=? AND expires_at>?`, signature, browserSignature, time.Now().UTC()).Scan(&p.Signature, &p.BrowserSignature, &payload, &p.ExpiresAt)
	if err != nil {
		return nil, oauthStoreError(err)
	}
	p.Payload = []byte(payload)
	return &p, nil
}
func (o *OAuthTransaction) ConsumePending(ctx context.Context, signature, browserSignature string) error {
	var found string
	err := o.tx.QueryRowContext(ctx, `DELETE FROM oauth_pending WHERE signature=? AND browser_signature=? AND expires_at>? RETURNING signature`, signature, browserSignature, time.Now().UTC()).Scan(&found)
	return oauthStoreError(err)
}

// Replay tombstones live through the absolute family lifetime; pruning is bounded and schema-derived.
func (o *OAuthTransaction) pruneExpiredGrants(ctx context.Context) error {
	rows, err := o.tx.QueryContext(ctx, "SELECT CAST(id AS VARCHAR) FROM oauth_grants WHERE expires_at<=? ORDER BY expires_at LIMIT 32", time.Now().UTC())
	if err != nil {
		return oauthStoreError(err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	plan, err := buildScopedDeletePlan(ctx, o.tx, scopedDeleteSpec{rootTable: "oauth_grants", scopeColumns: []string{"oauth_grant_id"}})
	if err != nil {
		return err
	}
	for _, id := range ids {
		for _, step := range plan {
			if _, err := o.tx.ExecContext(ctx, step.query, id); err != nil {
				return oauthStoreError(err)
			}
		}
		if _, err := o.tx.ExecContext(ctx, "DELETE FROM oauth_grants WHERE id=? AND expires_at<=?", id, time.Now().UTC()); err != nil {
			return oauthStoreError(err)
		}
	}
	return nil
}

// User deletion is policy-owned; OAuth child ordering still comes from the live schema.
func cleanupOAuthUserRows(ctx context.Context, tx *sql.Tx, userID string) error {
	tables, err := listTables(ctx, tx)
	if err != nil {
		return err
	}
	if _, exists := tables["oauth_grants"]; !exists {
		return nil
	}
	rows, err := tx.QueryContext(ctx, "SELECT CAST(id AS VARCHAR) FROM oauth_grants WHERE user_id=?", userID)
	if err != nil {
		return oauthStoreError(err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	plan, err := buildScopedDeletePlan(ctx, tx, scopedDeleteSpec{rootTable: "oauth_grants", scopeColumns: []string{"oauth_grant_id"}})
	if err != nil {
		return err
	}
	for _, id := range ids {
		for _, step := range plan {
			if _, err := tx.ExecContext(ctx, step.query, id); err != nil {
				return oauthStoreError(err)
			}
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM oauth_grants WHERE id=? AND user_id=?", id, userID); err != nil {
			return oauthStoreError(err)
		}
	}
	return nil
}

func oauthAuditParams(actorID, teamID, grantID uuid.UUID, action string) (AuditEntryParams, error) {
	switch action {
	case "oauth.consent_approved", "oauth.consent_denied", "oauth.grant_revoked", "oauth.refresh_reused", "oauth.code_reused":
	default:
		return AuditEntryParams{}, errors.New("invalid OAuth audit action")
	}
	params := AuditEntryParams{ActorID: googleuuid.UUID(actorID), TeamID: googleuuid.UUID(teamID), Action: action, TargetType: "oauth_grant", Outcome: "success"}
	if grantID == uuid.Nil() {
		params.TargetType = "oauth_consent"
	} else {
		params.TargetID = grantID.String()
	}
	return params, nil
}

func (o *OAuthTransaction) AppendAudit(ctx context.Context, actorID, teamID, grantID uuid.UUID, action string) error {
	params, err := oauthAuditParams(actorID, teamID, grantID, action)
	if err != nil {
		return err
	}
	return appendAuditEntryTx(ctx, o.tx, params)
}

// Reuse revocation commits before its audit so an audit failure cannot resurrect credentials.
func (s *Store) AppendOAuthReuseAudit(ctx context.Context, actorID, teamID, grantID uuid.UUID, refresh bool) error {
	action := "oauth.code_reused"
	if refresh {
		action = "oauth.refresh_reused"
	}
	params, err := oauthAuditParams(actorID, teamID, grantID, action)
	if err != nil {
		return err
	}
	return s.AppendAuditEntry(ctx, params)
}
