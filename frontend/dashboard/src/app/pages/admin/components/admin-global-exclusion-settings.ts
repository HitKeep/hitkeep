import { ChangeDetectionStrategy, Component } from '@angular/core';
import { ExclusionEditor } from '@features/exclusions/exclusion-editor';
import { AdminPageFrame } from './admin-page-frame';

@Component({
    selector: 'app-admin-global-exclusion-settings',
    imports: [AdminPageFrame, ExclusionEditor],
    template: `<app-admin-page-frame><app-exclusion-editor [scope]="{ kind: 'instance' }" /></app-admin-page-frame>`,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AdminGlobalExclusionSettings {}
