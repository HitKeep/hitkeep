import { ChangeDetectionStrategy, Component, inject, input } from '@angular/core';
import { NavigationSearch } from './navigation-search';
import { Brand } from '@components/brand/brand';
import { TranslocoPipe } from '@jsverse/transloco';
import { TeamSwitcher } from '@components/team-switcher/team-switcher';
import { UserControls } from '@components/user-controls/user-controls';
import { SiteSelector } from '@features/sites/components/site-selector';
import { MainLayoutContextService } from '@layout/main-layout-context.service';

@Component({
    selector: 'app-layout-page-bar',
    imports: [NavigationSearch, Brand, TeamSwitcher, UserControls, SiteSelector, TranslocoPipe],
    templateUrl: './layout-page-bar.html',
    styleUrl: './layout-page-bar.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class LayoutPageBar {
    readonly desktop = input(true);
    protected readonly context = inject(MainLayoutContextService);
    protected readonly shareService = this.context.shareService;
    protected readonly siteService = this.context.siteService;
    protected readonly teamService = this.context.teamService;
    protected readonly isCreateTeamVisible = this.context.isCreateTeamVisible;
    protected readonly beforeTeamSwitch = this.context.beforeTeamSwitch;
    protected readonly canCreateTeams = this.context.canCreateTeams;
}
