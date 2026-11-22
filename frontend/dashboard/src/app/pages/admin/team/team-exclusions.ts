import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { ExclusionEditor } from '@features/exclusions/exclusion-editor';
import { TeamService } from '@services/team.service';

@Component({
    selector: 'app-team-exclusions',
    imports: [ExclusionEditor],
    template: `@if (activeTeam(); as team) {
        <app-exclusion-editor [scope]="{ kind: 'team', id: team.id, name: team.name }" />
    }`,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class TeamExclusionsPage {
    protected readonly activeTeam = inject(TeamService).activeTeam;
}
