import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of, Subject } from 'rxjs';
import { vi } from 'vitest';

import { OAuthGrant } from '@models/oauth.types';
import { OAuthService } from '@services/oauth.service';
import { ConnectedApplicationsPage } from './connected-applications';

const grant: OAuthGrant = {
    id: 'grant-1',
    client: { id: 'client-1', name: 'Reporting app' },
    team: { id: 'team-1', name: 'Team' },
    sites: [{ id: 'site-1', domain: 'site.example' }],
    scopes: ['analytics:read'],
    resource: 'https://hitkeep.example/mcp',
    created_at: '2026-01-01T00:00:00Z',
    expires_at: '2027-01-01T00:00:00Z'
};

describe('ConnectedApplicationsPage', () => {
    let fixture: ComponentFixture<ConnectedApplicationsPage>;
    let deletion: Subject<void>;
    const oauth = { grants: vi.fn(), revoke: vi.fn() };

    beforeEach(async () => {
        deletion = new Subject<void>();
        oauth.grants.mockReset().mockReturnValue(of([grant]));
        oauth.revoke.mockReset().mockReturnValue(deletion);
        TestBed.configureTestingModule({
            imports: [ConnectedApplicationsPage, TranslocoTestingModule.forRoot({ langs: { en: {} }, translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }, preloadLangs: true })],
            providers: [provideRouter([]), { provide: OAuthService, useValue: oauth }]
        });
        fixture = TestBed.createComponent(ConnectedApplicationsPage);
        await fixture.whenStable();
    });

    it('opens connection metadata in a dialog and returns to the compact list on close', async () => {
        expect(fixture.nativeElement.querySelector('app-table')).not.toBeNull();
        expect(fixture.nativeElement.textContent).toContain('Reporting app');
        expect(fixture.nativeElement.textContent).not.toContain('https://hitkeep.example/mcp');
        const page = fixture.componentInstance as unknown as { openDetails(grant: OAuthGrant): void };
        page.openDetails(grant);
        await fixture.whenStable();
        const dialog = document.body.querySelector('[role="dialog"]') as HTMLElement;
        expect(dialog).not.toBeNull();
        expect(dialog.textContent).toContain('Reporting app');
        expect(dialog.textContent).toContain('https://hitkeep.example/mcp');
        expect(dialog.textContent).toContain('client-1');
        expect(dialog.textContent).toContain('analytics:read');
        const close = Array.from(dialog.querySelectorAll('button')).find((button) => button.textContent?.includes('common.actions.close'));
        expect(close).toBeDefined();
        close!.click();
        await fixture.whenStable();
        expect(document.body.querySelector('[role="dialog"]')).toBeNull();
        expect(fixture.nativeElement.textContent).not.toContain('https://hitkeep.example/mcp');
    });

    it('keeps the grant visible during revoke and removes it only after success', async () => {
        fixture.componentInstance['openDetails'](grant);
        await fixture.whenStable();
        const dialog = document.body.querySelector('[role="dialog"]') as HTMLElement;
        const button = Array.from(dialog.querySelectorAll('button')).find((item) => item.textContent?.includes('oauth.revoke'))!;
        button.click();
        await fixture.whenStable();
        expect(button.disabled).toBe(true);
        expect(fixture.nativeElement.querySelector('app-table')?.textContent).toContain('Reporting app');
        expect(oauth.revoke).toHaveBeenCalledExactlyOnceWith('grant-1');
        deletion.next();
        deletion.complete();
        await fixture.whenStable();
        expect(fixture.nativeElement.querySelector('app-table')?.textContent).not.toContain('Reporting app');
        expect(document.body.querySelector('[role="dialog"]')).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('oauth.revoked');
        expect(fixture.nativeElement.textContent).toContain('oauth.noApplications');
    });

    it('does not close another connection when an earlier revoke completes', async () => {
        const other: OAuthGrant = { ...grant, id: 'grant-2', client: { id: 'client-2', name: 'Another app' } };
        fixture.componentInstance['grants'].set([grant, other]);
        fixture.componentInstance['revoke'](grant);
        fixture.componentInstance['openDetails'](other);
        await fixture.whenStable();
        deletion.next();
        deletion.complete();
        await fixture.whenStable();
        const dialog = document.body.querySelector('[role="dialog"]');
        expect(dialog?.textContent).toContain('Another app');
        expect(dialog?.textContent).not.toContain('Reporting app');
        expect(fixture.nativeElement.querySelector('app-table')?.textContent).toContain('Another app');
        expect(fixture.nativeElement.querySelector('app-table')?.textContent).not.toContain('Reporting app');
    });

    it('preserves the grant and allows retry after revoke failure', async () => {
        fixture.componentInstance['openDetails'](grant);
        await fixture.whenStable();
        const dialog = document.body.querySelector('[role="dialog"]') as HTMLElement;
        const button = Array.from(dialog.querySelectorAll('button')).find((item) => item.textContent?.includes('oauth.revoke'))!;
        button.click();
        deletion.error(new Error('outage'));
        await fixture.whenStable();
        expect(button.disabled).toBe(false);
        expect(fixture.nativeElement.querySelector('app-table')?.textContent).toContain('Reporting app');
        expect(dialog.textContent).toContain('oauth.revokeFailed');
        expect(dialog.textContent).toContain('Reporting app');
    });
});
