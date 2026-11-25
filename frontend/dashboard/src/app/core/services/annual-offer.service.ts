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

    readonly offer = computed(() => {
        const state = this.state.hasValue() ? this.state.value() : null;
        return state?.plan_code === 'free' && state.team_id === this.teams.activeTeam()?.id ? state.annual_offer : undefined;
    });
}
