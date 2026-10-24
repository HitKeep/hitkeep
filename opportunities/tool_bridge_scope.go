package opportunities

import (
	"context"
	"fmt"
	stduuid "uuid"

	"github.com/google/uuid"

	hitai "hitkeep/ai"
	"hitkeep/auth"
)

type toolBridgeScope struct {
	config ToolBridgeConfig
}

func newToolBridgeScope(config ToolBridgeConfig) toolBridgeScope {
	return toolBridgeScope{config: config}
}

func (s toolBridgeScope) authorize(ctx context.Context) error {
	if s.config.Shared == nil || s.config.Analytics == nil {
		return fmt.Errorf("tool bridge unavailable")
	}
	if err := s.authorizeConfiguredSiteScope(ctx); err != nil {
		return err
	}
	switch s.config.ActorType {
	case "ai_scheduler":
		return s.authorizeScheduler()
	case "api_client":
		return s.authorizeAPIClient(ctx)
	default:
		return s.authorizeUser(ctx)
	}
}

func (s toolBridgeScope) authorizeConfiguredSiteScope(ctx context.Context) error {
	resource, err := s.config.Shared.GetSiteResource(ctx, stduuid.UUID(s.config.SiteID))
	if err != nil || resource.TeamID != stduuid.UUID(s.config.TeamID) {
		return hitai.ErrAccessDenied
	}
	return nil
}

func (s toolBridgeScope) authorizeScheduler() error {
	if s.config.SchedulerTeamID != s.config.TeamID || s.config.SchedulerSiteID != s.config.SiteID {
		return hitai.ErrAccessDenied
	}
	return nil
}

func (s toolBridgeScope) authorizeUser(ctx context.Context) error {
	if s.config.ActorID == uuid.Nil {
		return hitai.ErrAccessDenied
	}
	if s.config.APIClientAuth != nil {
		return hitai.ErrAccessDenied
	}
	if s.config.EffectiveUserID != s.config.ActorID {
		return hitai.ErrAccessDenied
	}
	if !s.hasEffectiveSiteView() {
		return hitai.ErrAccessDenied
	}
	instanceRole, err := s.config.Shared.GetInstanceRole(ctx, s.config.ActorID)
	if err != nil {
		return hitai.ErrAccessDenied
	}
	if instanceRole.HasPermission(auth.PermSiteView) {
		return nil
	}
	siteRole, err := s.config.Shared.GetSiteRoleForTenant(ctx, stduuid.UUID(s.config.ActorID), stduuid.UUID(s.config.SiteID), stduuid.UUID(s.config.TeamID))
	if err != nil || !siteRole.HasPermission(auth.PermSiteView) {
		return hitai.ErrAccessDenied
	}
	return nil
}

func (s toolBridgeScope) authorizeAPIClient(ctx context.Context) error {
	apiClient := s.config.APIClientAuth
	if apiClient == nil || apiClient.ClientID == uuid.Nil || apiClient.ClientID != s.config.ActorID {
		return hitai.ErrAccessDenied
	}
	if apiClient.TenantID != uuid.Nil && apiClient.TenantID != s.config.TeamID {
		return hitai.ErrAccessDenied
	}
	if apiClient.UserID != s.config.EffectiveUserID {
		return hitai.ErrAccessDenied
	}
	if !s.hasEffectiveSiteView() {
		return hitai.ErrAccessDenied
	}
	delegatedRole, hasSiteDelegation := apiClient.SiteRoles[s.config.SiteID]
	if !hasSiteDelegation || !delegatedRole.HasPermission(auth.PermSiteView) {
		return hitai.ErrAccessDenied
	}
	current, err := s.config.Shared.RevalidateAPIClientAuth(ctx, apiClient)
	if err != nil || current == nil || current.UserID != apiClient.UserID || current.TenantID != apiClient.TenantID {
		return hitai.ErrAccessDenied
	}
	if role, ok := current.SiteRoles[s.config.SiteID]; ok && role.HasPermission(auth.PermSiteView) {
		return nil
	}
	return hitai.ErrAccessDenied
}

func (s toolBridgeScope) hasEffectiveSiteView() bool {
	return s.config.EffectiveInstanceRole.HasPermission(auth.PermSiteView) ||
		s.config.EffectiveSiteRole.HasPermission(auth.PermSiteView)
}
