import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { ExclusionEditor } from '@features/exclusions/exclusion-editor';
import { SiteService } from '@features/sites/services/site.service';

@Component({
    selector: 'app-site-filtering-settings-page',
    imports: [ExclusionEditor],
    template: `@if (activeSite(); as site) {
        <app-exclusion-editor [scope]="{ kind: 'site', id: site.id, name: site.domain }" />
    }`,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SiteFilteringSettingsPage {
    protected readonly activeSite = inject(SiteService).activeSite;
}
