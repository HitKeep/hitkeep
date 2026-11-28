//go:build !billing

package entitlements

import (
	"context"

	"github.com/google/uuid"
)

func (s *Service) cloudBillingTeamPlan(_ context.Context, _ uuid.UUID) *PlanInfo {
	return nil
}

func (s *Service) cloudBillingTeamEntitlements(_ context.Context, _ uuid.UUID) *Entitlements {
	return nil
}

func (s *Service) AllowsCustomBranding(ctx context.Context, actorID, teamID uuid.UUID) bool {
	return true
}
