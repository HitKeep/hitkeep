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

    it('keeps the grant visible during revoke and removes it only after success', async () => {
        const button = fixture.nativeElement.querySelector('article button') as HTMLButtonElement;
        button.click();
        await fixture.whenStable();
        expect(button.disabled).toBe(true);
        expect(fixture.nativeElement.querySelector('article')).not.toBeNull();
        expect(oauth.revoke).toHaveBeenCalledExactlyOnceWith('grant-1');
        deletion.next();
        deletion.complete();
        await fixture.whenStable();
        expect(fixture.nativeElement.querySelector('article')).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('oauth.revoked');
        expect(fixture.nativeElement.textContent).toContain('oauth.noApplications');
    });

    it('preserves the grant and allows retry after revoke failure', async () => {
        const button = fixture.nativeElement.querySelector('article button') as HTMLButtonElement;
        button.click();
        deletion.error(new Error('outage'));
        await fixture.whenStable();
        expect(button.disabled).toBe(false);
        expect(fixture.nativeElement.querySelector('article')).not.toBeNull();
        expect(fixture.nativeElement.textContent).toContain('oauth.revokeFailed');
    });
});
