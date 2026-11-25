import { computed, inject, Service } from '@angular/core';
import { rxResource } from '@angular/core/rxjs-interop';
import { CloudService } from './cloud.service';
import { DashboardBootstrapService } from './dashboard-bootstrap.service';
import { ShareService } from './share.service';
import { TeamService } from './team.service';

@Service()
export class BillingPromotionService {
    private readonly bootstrap = inject(DashboardBootstrapService);
    private readonly share = inject(ShareService);
    private readonly teams = inject(TeamService);
    private readonly cloud = inject(CloudService);
    private readonly state = rxResource({
        params: () => {
            const team = this.teams.activeTeam();
            const plan = team?.plan?.code;
            return this.bootstrap.status()?.cloud?.promotion_configured && !this.share.isShareMode() && (plan === 'free' || plan === 'pro' || plan === 'business') && (team?.role === 'owner' || team?.role === 'admin') ? team.id : undefined;
        },
        stream: ({ params }) => this.cloud.watchBillingState(params)
    });

    private readonly plans = rxResource({
        params: () => (this.teams.activeTeam()?.plan?.code === 'free' ? this.offer()?.code : undefined),
        stream: () => this.cloud.getPlans()
    });

    readonly spotlight = computed(() => {
        const offer = this.offer();
        const team = this.teams.activeTeam();
        if (!offer || !team) return null;
        const paid = team.plan?.code === 'pro' || team.plan?.code === 'business';
        const plan = paid ? team.plan!.code : offer.prices['pro'] ? 'pro' : 'business';
        const prices = offer.prices[plan];
        const interval: 'annual' | 'monthly' = prices?.annual ? 'annual' : 'monthly';
        const price = prices?.[interval];
        if (!price) return null;
        const days = this.plans.hasValue() ? this.plans.value().find((tier) => tier.code === plan)?.entitlements.max_retention_days : undefined;
        return { paid, plan, interval, days, current: team.entitlements?.max_retention_days, teamId: team.id, offer, price };
    });

    readonly offer = computed(() => {
        const state = this.state.hasValue() ? this.state.value() : null;
        return state && state.team_id === this.teams.activeTeam()?.id ? state.promotion : undefined;
    });
}
