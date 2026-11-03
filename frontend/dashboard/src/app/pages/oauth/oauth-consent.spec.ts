import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { FormControl } from '@angular/forms';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { TranslocoService, TranslocoTestingModule } from '@jsverse/transloco';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';

import { OAuthConsent } from '@models/oauth.types';
import { OAuthService } from '@services/oauth.service';
import { OAuthConsentPage } from './oauth-consent';
import { UserProfileService } from '@services/user-profile.service';
import { UserMenuService } from '@services/user-menu.service';

const preview = (): OAuthConsent => ({
    client: { id: 'client-1', name: '<img src=x onerror=alert(1)>', callback_origin: 'https://client.example' },
    resource: 'https://hitkeep.example/mcp',
    scopes: ['analytics:read'],
    expires_at: new Date(Date.now() + 600_000).toISOString(),
    csrf_token: 'csrf-binding',
    teams: [
        { id: 'team-1', name: 'First team', sites: [{ id: 'site-1', domain: 'first.example' }] },
        { id: 'team-2', name: 'Second team', sites: [{ id: 'site-2', domain: 'second.example' }] }
    ]
});

interface ConsentControls {
    form: {
        team_id: () => { control: () => FormControl<string> };
        site_ids: () => { control: () => FormControl<string[]> };
    };
}

describe('OAuthConsentPage', () => {
    let fixture: ComponentFixture<OAuthConsentPage>;
    const oauth = { consent: vi.fn(), createSite: vi.fn() };
    const accountMenu = { signOut: vi.fn() };

    beforeEach(() => {
        oauth.consent.mockReset().mockReturnValue(of(preview()));
        oauth.createSite.mockReset();
        accountMenu.signOut.mockClear();
        TestBed.configureTestingModule({
            imports: [
                OAuthConsentPage,
                TranslocoTestingModule.forRoot({
                    langs: { en: { oauth: { scope: { analyticsRead: 'Read analytics' } } }, de: { oauth: { scope: { analyticsRead: 'Analytics lesen' } } } },
                    translocoConfig: { availableLangs: ['en', 'de'], defaultLang: 'en', reRenderOnLangChange: true },
                    preloadLangs: true
                })
            ],
            providers: [provideRouter([]), { provide: OAuthService, useValue: oauth }, { provide: UserProfileService, useValue: { profile: signal({ email: 'user@example.test' }) } }, { provide: UserMenuService, useValue: accountMenu }]
        });
    });

    async function create() {
        fixture = TestBed.createComponent(OAuthConsentPage);
        await fixture.whenStable();
        return (fixture.componentInstance as unknown as ConsentControls).form;
    }

    it('requires a team and explicit eligible sites and clears selections on team changes', async () => {
        const form = await create();
        const approve = () => fixture.nativeElement.querySelector('button[value="approve"]') as HTMLButtonElement;
        expect(approve().disabled).toBe(true);
        form.team_id().control().setValue('team-1');
        form.site_ids().control().setValue(['site-1', 'site-2']);
        await fixture.whenStable();
        expect(approve().disabled).toBe(false);
        expect([...fixture.nativeElement.querySelectorAll('input[name="site_ids"]')].map((input) => (input as HTMLInputElement).value)).toEqual(['site-1']);
        form.team_id().control().setValue('team-2');
        await fixture.whenStable();
        expect(approve().disabled).toBe(true);
        expect(fixture.nativeElement.querySelectorAll('input[name="site_ids"]').length).toBe(0);
    });

    it('lets a new account create its first site and then explicitly approve the connection', async () => {
        const emptyConsent = { ...preview(), suggested_domain: 'wordpress.example', teams: [{ id: 'team-1', name: 'First team', sites: [], can_create_site: true }] };
        oauth.consent.mockReturnValueOnce(of(emptyConsent)).mockReturnValueOnce(of({ ...emptyConsent, teams: [{ ...emptyConsent.teams[0], sites: [{ id: 'new-site', domain: 'wordpress.example' }] }] }));
        oauth.createSite.mockReturnValue(of({ id: 'new-site', domain: 'wordpress.example' }));
        await create();
        const input = fixture.nativeElement.querySelector('#oauth-site-domain') as HTMLInputElement;
        expect(input.value).toBe('wordpress.example');
        expect(oauth.createSite).not.toHaveBeenCalled();
        expect((fixture.nativeElement.querySelector('button[value="approve"]') as HTMLButtonElement).disabled).toBe(true);
        const createButton = fixture.nativeElement.querySelector('section[aria-labelledby="oauth-create-site-title"] button') as HTMLButtonElement;
        createButton.click();
        await fixture.whenStable();
        expect(oauth.createSite).toHaveBeenCalledWith('team-1', 'csrf-binding', 'wordpress.example');
        expect([...fixture.nativeElement.querySelectorAll('input[name="site_ids"]')].map((input) => (input as HTMLInputElement).value)).toEqual(['new-site']);
        expect((fixture.nativeElement.querySelector('button[value="approve"]') as HTMLButtonElement).disabled).toBe(false);
        expect(fixture.nativeElement.querySelector('input[name="decision"]').value).toBe('');
    });

    it('shows safe site creation errors and preserves the consent form for retry', async () => {
        oauth.consent.mockReturnValue(of({ ...preview(), teams: [{ id: 'team-1', name: 'First team', sites: [], can_create_site: true }], suggested_domain: 'wordpress.example' }));
        oauth.createSite.mockReturnValue(throwError(() => ({ status: 409, message: 'private details' })));
        await create();
        (fixture.nativeElement.querySelector('section[aria-labelledby="oauth-create-site-title"] button') as HTMLButtonElement).click();
        await fixture.whenStable();
        expect(fixture.nativeElement.textContent).toContain('oauth.siteCreateExists');
        expect(fixture.nativeElement.textContent).not.toContain('private details');
        expect(fixture.nativeElement.querySelector('form')).not.toBeNull();
    });

    it('does not offer site creation to a team without creation permission', async () => {
        const controls = await create();
        controls.team_id().control().setValue('team-1');
        await fixture.whenStable();
        expect(fixture.nativeElement.querySelector('#oauth-site-domain')).toBeNull();
    });

    it('renders untrusted identity as text and posts denial without requiring site selection', async () => {
        await create();
        expect(fixture.nativeElement.textContent).toContain('<img src=x onerror=alert(1)>');
        expect(fixture.nativeElement.querySelector('dl img')).toBeNull();
        const form = fixture.nativeElement.querySelector('form') as HTMLFormElement;
        const deny = form.querySelector('button[value="deny"]') as HTMLButtonElement;
        const event = new SubmitEvent('submit', { submitter: deny, bubbles: true, cancelable: true });
        form.dispatchEvent(event);
        expect(event.defaultPrevented).toBe(false);
        expect(new FormData(form).getAll('decision')).toEqual(['deny']);
        expect(form.method).toBe('post');
        expect(new URL(form.action).pathname).toContain('/api/oauth/consent');
        expect(form.querySelector('input[name="csrf_token"]')).not.toBeNull();
        const repeated = new SubmitEvent('submit', { submitter: deny, bubbles: true, cancelable: true });
        form.dispatchEvent(repeated);
        expect(repeated.defaultPrevented).toBe(true);
    });

    it('preserves the explicit approval payload before the native submission', async () => {
        const controls = await create();
        controls.team_id().control().setValue('team-1');
        controls.site_ids().control().setValue(['site-1']);
        await fixture.whenStable();
        const form = fixture.nativeElement.querySelector('form') as HTMLFormElement;
        const button = form.querySelector('button[value="approve"]') as HTMLButtonElement;
        const event = new SubmitEvent('submit', { submitter: button, bubbles: true, cancelable: true });
        form.dispatchEvent(event);
        await fixture.whenStable();
        const payload = new FormData(form);
        expect(event.defaultPrevented).toBe(false);
        expect(payload.getAll('decision')).toEqual(['approve']);
        expect(payload.get('team_id')).toBe('team-1');
        expect(payload.getAll('site_ids')).toEqual(['site-1']);
        expect(payload.get('csrf_token')).toBe('csrf-binding');
    });

    it('authorizes team-only billing access without inventing a site selection', async () => {
        oauth.consent.mockReturnValue(of({ ...preview(), scopes: ['billing:write'], teams: [{ id: 'team-1', name: 'Billing team', sites: [] }] }));
        const controls = await create();
        const approve = () => fixture.nativeElement.querySelector('button[value="approve"]') as HTMLButtonElement;
        expect(approve().disabled).toBe(true);
        controls.team_id().control().setValue('team-1');
        await fixture.whenStable();
        expect(approve().disabled).toBe(false);
        expect(fixture.nativeElement.querySelector('.oauth-sites')).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('oauth.billingHelp');
        const form = fixture.nativeElement.querySelector('form') as HTMLFormElement;
        const event = new SubmitEvent('submit', { submitter: approve(), bubbles: true, cancelable: true });
        form.dispatchEvent(event);
        expect(event.defaultPrevented).toBe(false);
        const payload = new FormData(form);
        expect(payload.get('team_id')).toBe('team-1');
        expect(payload.getAll('site_ids')).toEqual([]);
        expect(payload.get('decision')).toBe('approve');
    });

    it('still requires explicit sites when billing and site permissions are requested together', async () => {
        oauth.consent.mockReturnValue(of({ ...preview(), scopes: ['billing:write', 'analytics:read'] }));
        const controls = await create();
        controls.team_id().control().setValue('team-1');
        await fixture.whenStable();
        const approve = () => fixture.nativeElement.querySelector('button[value="approve"]') as HTMLButtonElement;
        expect(approve().disabled).toBe(true);
        controls.site_ids().control().setValue(['site-1']);
        await fixture.whenStable();
        expect(approve().disabled).toBe(false);
    });

    it('does not expose an approval form for expired requests', async () => {
        oauth.consent.mockReturnValue(of({ ...preview(), expires_at: new Date(Date.now() - 1000).toISOString() }));
        await create();
        expect(fixture.nativeElement.querySelector('form')).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('oauth.expired');
    });

    it('expires a loaded request while the user is choosing sites', async () => {
        vi.useFakeTimers({ toFake: ['Date', 'setInterval', 'clearInterval'] });
        try {
            const controls = await create();
            controls.team_id().control().setValue('team-1');
            controls.site_ids().control().setValue(['site-1']);
            await fixture.whenStable();
            expect(fixture.nativeElement.querySelector('form')).not.toBeNull();
            vi.advanceTimersByTime(600_001);
            await fixture.whenStable();
            expect(fixture.nativeElement.querySelector('form')).toBeNull();
            expect(fixture.nativeElement.textContent).toContain('oauth.expired');
        } finally {
            fixture.destroy();
            vi.useRealTimers();
        }
    });

    it('treats native form error codes as display only and keeps GET authoritative', async () => {
        TestBed.overrideProvider(ActivatedRoute, { useValue: { snapshot: { queryParamMap: convertToParamMap({ error: 'expired' }) } } });
        await create();
        expect(fixture.nativeElement.textContent).toContain('oauth.expired');
        expect(fixture.nativeElement.querySelector('form')).not.toBeNull();
        expect(oauth.consent).toHaveBeenCalledOnce();
    });

    it('does not render unknown error parameters', async () => {
        TestBed.overrideProvider(ActivatedRoute, { useValue: { snapshot: { queryParamMap: convertToParamMap({ error: 'private-server-details' }) } } });
        await create();
        expect(fixture.nativeElement.textContent).not.toContain('private-server-details');
        expect(fixture.nativeElement.querySelector('form')).not.toBeNull();
    });

    it('updates readable abilities when the language changes', async () => {
        await create();
        expect(fixture.nativeElement.textContent).toContain('Read analytics');
        TestBed.inject(TranslocoService).setActiveLang('de');
        await fixture.whenStable();
        expect(fixture.nativeElement.textContent).toContain('Analytics lesen');
    });

    it('keeps readable consent and account switching without dashboard controls', async () => {
        await create();
        expect(fixture.nativeElement.querySelector('app-user-controls')).toBeNull();
        expect(fixture.nativeElement.querySelector('.oauth-permission-list').textContent).toContain('Read analytics');
        expect(fixture.nativeElement.querySelector('.oauth-permission-list').textContent).not.toContain('analytics:read');
        const details = fixture.nativeElement.querySelector('details') as HTMLDetailsElement;
        expect(details.open).toBe(false);
        expect(details.textContent).toContain('analytics:read');
        expect(details.textContent).toContain('https://hitkeep.example/mcp');
        const signOut = fixture.nativeElement.querySelector('.oauth-sign-out') as HTMLButtonElement;
        expect(signOut.type).toBe('button');
        signOut.click();
        expect(accountMenu.signOut).toHaveBeenCalledOnce();
    });

    it.each([
        [403, 'oauth.permissionDenied'],
        [410, 'oauth.expired'],
        [503, 'oauth.loadFailed']
    ])('shows a safe error for status %s', async (status, key) => {
        oauth.consent.mockReturnValue(throwError(() => ({ status })));
        await create();
        expect(fixture.nativeElement.textContent).toContain(key);
        expect(fixture.nativeElement.querySelector('form')).toBeNull();
    });
});
