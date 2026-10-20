package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"uuid"

	legacyuuid "github.com/google/uuid"

	"hitkeep/auth"
)

var errSiteAccessDenied = errors.New("no access to site")

// GetSiteResource resolves current ownership without the ingestion routing cache.
func (s *Store) GetSiteResource(ctx context.Context, siteID uuid.UUID) (auth.Resource, error) {
	teamID, err := s.querySiteTenantID(ctx, legacyuuid.UUID(siteID))
	if err != nil {
		return auth.Resource{}, err
	}
	var archived bool
	if err := s.db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM tenant_archives WHERE tenant_id = ?)", teamID).Scan(&archived); err != nil {
		return auth.Resource{}, err
	}
	if archived {
		return auth.Resource{}, errSiteAccessDenied
	}
	return auth.Resource{Kind: auth.ResourceSite, ID: siteID, TeamID: uuid.UUID(teamID)}, nil
}

// GetSiteRoleForTenant resolves current consent-team authority independently of
// the dashboard selection and runtime caches.
func (s *Store) GetSiteRoleForTenant(ctx context.Context, userID, siteID, tenantID uuid.UUID) (auth.SiteRole, error) {
	if userID == uuid.Nil() || siteID == uuid.Nil() || tenantID == uuid.Nil() {
		return "", errSiteAccessDenied
	}
	member, err := s.IsTenantMember(ctx, legacyuuid.UUID(tenantID), legacyuuid.UUID(userID))
	if err != nil {
		return "", err
	}
	if !member {
		return "", errSiteAccessDenied
	}
	return s.getSiteRoleForTenant(ctx, userID, siteID, tenantID)
}

// getSiteRoleForTenant translates existing compact role bindings without
// changing legacy dashboard membership or inheritance semantics.
func (s *Store) getSiteRoleForTenant(ctx context.Context, userID, siteID, tenantID uuid.UUID) (auth.SiteRole, error) {
	defaultTenantID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to resolve default tenant: %w", err)
	}
	// Existing SQL scanners and driver bindings use the legacy UUID type.
	queryUser, querySite, queryTenant := legacyuuid.UUID(userID), legacyuuid.UUID(siteID), legacyuuid.UUID(tenantID)
	var explicitRole string
	err = s.db.QueryRowContext(ctx, `
		SELECT sm.role FROM site_members sm
		JOIN sites s ON s.id = sm.site_id
		LEFT JOIN site_tenants st ON st.site_id = s.id
		WHERE sm.user_id = ? AND sm.site_id = ? AND COALESCE(st.tenant_id, ?) = ?
	`, queryUser, querySite, defaultTenantID, queryTenant).Scan(&explicitRole)
	hasExplicitRole := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("failed to get site role: %w", err)
	}
	var ownerID legacyuuid.UUID
	err = s.db.QueryRowContext(ctx, `
		SELECT user_id FROM sites s LEFT JOIN site_tenants st ON st.site_id = s.id
		WHERE s.id = ? AND COALESCE(st.tenant_id, ?) = ?
	`, querySite, defaultTenantID, queryTenant).Scan(&ownerID)
	isOwner := err == nil && ownerID == queryUser
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("failed to resolve site owner: %w", err)
	}
	var tenantRole string
	err = s.db.QueryRowContext(ctx, `
		SELECT tm.role FROM sites s
		LEFT JOIN site_tenants st ON st.site_id = s.id
		JOIN tenant_members tm ON tm.tenant_id = COALESCE(st.tenant_id, ?) AND tm.user_id = ?
		LEFT JOIN tenant_archives ta ON ta.tenant_id = tm.tenant_id
		WHERE s.id = ? AND COALESCE(st.tenant_id, ?) = ? AND ta.tenant_id IS NULL
	`, defaultTenantID, queryUser, querySite, defaultTenantID, queryTenant).Scan(&tenantRole)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("failed to resolve tenant site role: %w", err)
	}
	return effectiveSiteRole(sql.NullString{String: explicitRole, Valid: hasExplicitRole}, isOwner, tenantRole)
}

// effectiveSiteRole keeps scalar and batched binding reads on the same role policy.
func effectiveSiteRole(explicitRole sql.NullString, isOwner bool, tenantRole string) (auth.SiteRole, error) {
	var effectiveRole auth.SiteRole
	hasEffectiveRole := false
	mergeRole := func(role auth.SiteRole) {
		if !hasEffectiveRole {
			effectiveRole, hasEffectiveRole = role, true
			return
		}
		effectiveRole = auth.MinSiteRole(effectiveRole, role)
	}
	if explicitRole.Valid {
		mergeRole(auth.SiteRole(explicitRole.String))
	}
	if isOwner {
		mergeRole(auth.SiteOwner)
	}
	if role, ok := tenantRoleSiteRole(tenantRole); ok {
		mergeRole(role)
	}
	if !hasEffectiveRole {
		return "", errSiteAccessDenied
	}
	return effectiveRole, nil
}
