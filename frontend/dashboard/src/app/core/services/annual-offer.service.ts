import { computed, inject, Service } from '@angular/core';
import { rxResource } from '@angular/core/rxjs-interop';
import { CloudService, billingStateUntilExpiry } from './cloud.service';
import { DashboardBootstrapService } from './dashboard-bootstrap.service';
import { ShareService } from './share.service';
import { TeamService } from './team.service';

@Service()
export class AnnualOfferService {
    private readonly bootstrap = inject(DashboardBootstrapService);
    private readonly share = inject(ShareService);
    private readonly teams = inject(TeamService);
    private readonly cloud = inject(CloudService);
    private readonly state = rxResource({
        params: () => {
            const team = this.teams.activeTeam();
            return this.bootstrap.cloudHosted() && !this.share.isShareMode() && team?.plan?.code === 'free' && (team.role === 'owner' || team.role === 'admin') ? team.id : undefined;
        },
        stream: () => billingStateUntilExpiry(this.cloud.getBillingState())
    });

    private readonly plans = rxResource({
        params: () => this.offer()?.code,
        stream: () => this.cloud.getPlans()
    });

    readonly spotlight = computed(() => {
        const offer = this.offer();
        const team = this.teams.activeTeam();
        if (!offer || !team || !this.plans.hasValue()) return null;
        const plan = offer.prices['pro'] ? 'pro' : 'business';
        const tier = this.plans.value().find((candidate) => candidate.code === plan);
        const days = tier?.entitlements.max_retention_days;
        const current = team.entitlements?.max_retention_days;
        if (!days || !current || !offer.prices[plan]) return null;
        return { plan, days, current, teamId: team.id, endsAt: offer.ends_at, price: offer.prices[plan] };
    });

    readonly offer = computed(() => {
        const state = this.state.hasValue() ? this.state.value() : null;
        return state?.plan_code === 'free' && state.team_id === this.teams.activeTeam()?.id ? state.annual_offer : undefined;
    });
}
