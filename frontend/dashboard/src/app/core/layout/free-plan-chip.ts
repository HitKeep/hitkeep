import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';

import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { ShareService } from '@services/share.service';
import { TeamService } from '@services/team.service';

/** Header link reminding free cloud teams of their plan, linking to the plan comparison. */
@Component({
    selector: 'app-free-plan-chip',
    imports: [RouterLink, TranslocoPipe],
    template: `
        @if (visible()) {
            <a class="free-plan-chip" routerLink="/admin/team/overview" [attr.aria-label]="'cloud.planChip.aria' | transloco">
                <span>{{ 'cloud.planChip.free' | transloco }}</span>
            </a>
        }
    `,
    styles: [
        `
            :host {
                display: block;
            }

            .free-plan-chip {
                white-space: nowrap;
                display: inline-flex;
                align-items: center;
                gap: 0.4rem;
                padding: 0.3rem 0.65rem;
                border: 1px solid var(--p-content-border-color);
                border-radius: 999px;
                color: var(--p-text-muted-color);
                font-size: 0.75rem;
                font-weight: 650;
                line-height: 1.2;
                text-decoration: none;
                transition:
                    border-color 160ms ease,
                    color 160ms ease;
            }

            .free-plan-chip:hover,
            .free-plan-chip:focus-visible {
                border-color: color-mix(in srgb, var(--p-primary-color) 40%, var(--p-content-border-color));
                color: var(--p-primary-color);
            }
        `
    ],
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class FreePlanChip {
    private readonly bootstrap = inject(DashboardBootstrapService);
    private readonly share = inject(ShareService);
    private readonly teamService = inject(TeamService);

    protected readonly visible = computed(() => Boolean(this.bootstrap.cloudHosted() && !this.share.isShareMode() && this.teamService.activeTeam()?.plan?.code === 'free'));
}
