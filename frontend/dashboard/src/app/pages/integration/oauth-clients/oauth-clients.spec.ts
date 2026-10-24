import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal, WritableSignal } from '@angular/core';
import { FormControl, FormGroup } from '@angular/forms';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { of, Subject } from 'rxjs';
import { vi } from 'vitest';

import { OAuthService } from '@services/oauth.service';
import { TeamService } from '@services/team.service';
import { AccessService } from '@services/access.service';
import { OAuthClientRegistration } from '@models/oauth.types';
import { OAuthClientsPage } from './oauth-clients';

const client: OAuthClientRegistration = { client_id: 'hk_client_1', client_name: 'Reporting app', redirect_uris: ['https://reports.example/callback'] };
interface PageActions {
    open(client?: OAuthClientRegistration): void;
    submit(): void;
    confirmRevoke(client: OAuthClientRegistration): void;
    visible: WritableSignal<boolean>;
    form: FormGroup<{ name: FormControl<string>; uri: FormControl<string>; redirects: FormControl<string> }>;
}

describe('OAuthClientsPage', () => {
    let fixture: ComponentFixture<OAuthClientsPage>;
    let page: PageActions;
    const team = signal({ id: 'team-1' });
    const allowed = signal(true);
    const oauth = { clients: vi.fn(), saveClient: vi.fn(), revokeClient: vi.fn() };

    beforeEach(async () => {
        team.set({ id: 'team-1' });
        allowed.set(true);
        oauth.clients.mockReset().mockReturnValue(of([client]));
        oauth.saveClient.mockReset();
        oauth.revokeClient.mockReset();
        TestBed.configureTestingModule({
            imports: [OAuthClientsPage, TranslocoTestingModule.forRoot({ langs: { en: {} }, translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }, preloadLangs: true })],
            providers: [provideRouter([]), { provide: OAuthService, useValue: oauth }, { provide: TeamService, useValue: { activeTeam: team } }, { provide: AccessService, useValue: { canActiveTeam: () => allowed() } }]
        });
        fixture = TestBed.createComponent(OAuthClientsPage);
        page = fixture.componentInstance as unknown as PageActions;
        await fixture.whenStable();
    });

    it('opens registration details separately from editing', async () => {
        expect(fixture.nativeElement.textContent).not.toContain('https://reports.example/callback');
        const details = fixture.componentInstance as unknown as { showDetails(client: OAuthClientRegistration): void };
        details.showDetails(client);
        await fixture.whenStable();
        const dialog = document.body.querySelector('[role="dialog"]') as HTMLElement;
        expect(dialog).not.toBeNull();
        expect(dialog.textContent).toContain('Reporting app');
        expect(dialog.textContent).toContain('hk_client_1');
        expect(dialog.textContent).toContain('https://reports.example/callback');
        expect(document.getElementById('oauth-client-name')).toBeNull();
        const edit = Array.from(dialog.querySelectorAll('button')).find((button) => button.textContent?.includes('common.actions.edit'));
        expect(edit).toBeDefined();
        edit!.click();
        await fixture.whenStable();
        expect(page.visible()).toBe(true);
        expect(document.getElementById('oauth-client-name')).not.toBeNull();
        expect(document.body.querySelectorAll('[role="dialog"]')).toHaveLength(1);
    });

    it('saves exact redirects and shows the generated client ID only after success', async () => {
        const response = new Subject<OAuthClientRegistration>();
        oauth.saveClient.mockReturnValue(response);
        page.open();
        page.form.setValue({ name: 'New app', uri: '', redirects: 'https://new.example/callback\nhttps://new.example/second' });
        page.submit();
        await fixture.whenStable();
        expect(oauth.saveClient).toHaveBeenCalledExactlyOnceWith('team-1', null, { client_name: 'New app', client_uri: '', redirect_uris: ['https://new.example/callback', 'https://new.example/second'] });
        expect(page.visible()).toBe(true);
        response.next({ ...client, client_id: 'new-generated-id', client_name: 'New app' });
        response.complete();
        await fixture.whenStable();
        expect(page.visible()).toBe(false);
        expect(fixture.nativeElement.textContent).toContain('new-generated-id');
        expect(fixture.nativeElement.textContent).toContain('oauthClients.saved');
    });

    it('closes registration details on team or capability changes', async () => {
        fixture.componentInstance['showDetails'](client);
        await fixture.whenStable();
        expect(document.body.querySelector('[role="dialog"]')?.textContent).toContain('hk_client_1');
        team.set({ id: 'team-2' });
        await fixture.whenStable();
        expect(document.body.querySelector('[role="dialog"]')).toBeNull();
        fixture.componentInstance['showDetails'](client);
        await fixture.whenStable();
        allowed.set(false);
        await fixture.whenStable();
        expect(document.body.querySelector('[role="dialog"]')).toBeNull();
        expect(fixture.nativeElement.querySelector('app-table')).toBeNull();
    });

    it('keeps an edited registration available after save failure', async () => {
        const response = new Subject<OAuthClientRegistration>();
        oauth.saveClient.mockReturnValue(response);
        page.open(client);
        page.form.controls.name.setValue('Renamed app');
        page.submit();
        response.error(new Error('outage'));
        await fixture.whenStable();
        expect(oauth.saveClient.mock.calls[0].slice(0, 2)).toEqual(['team-1', client.client_id]);
        expect(page.visible()).toBe(true);
        expect(page.form.controls.name.value).toBe('Renamed app');
        expect(document.getElementById('oauth-client-name')?.closest('.p-dialog')?.textContent).toContain('oauthClients.saveFailed');
    });

    it('requires confirmation and retains the registration until revocation succeeds', async () => {
        const response = new Subject<void>();
        oauth.revokeClient.mockReturnValue(response);
        const confirmation = fixture.debugElement.injector.get(ConfirmationService);
        const confirm = vi.spyOn(confirmation, 'confirm');
        page.confirmRevoke(client);
        expect(oauth.revokeClient).not.toHaveBeenCalled();
        confirm.mock.calls[0][0].accept?.();
        await fixture.whenStable();
        expect(fixture.nativeElement.textContent).toContain(client.client_id);
        response.error(new Error('outage'));
        await fixture.whenStable();
        expect(fixture.nativeElement.textContent).toContain(client.client_id);
        expect(fixture.nativeElement.textContent).toContain('oauth.revokeFailed');
    });

    it('discards a previous team mutation and hides administration after capability loss', async () => {
        const response = new Subject<OAuthClientRegistration>();
        oauth.saveClient.mockReturnValue(response);
        page.open(client);
        page.submit();
        team.set({ id: 'team-2' });
        oauth.clients.mockReturnValue(of([]));
        await fixture.whenStable();
        response.next({ ...client, client_id: 'stale-registration' });
        response.complete();
        await fixture.whenStable();
        expect(fixture.nativeElement.textContent).not.toContain('stale-registration');
        allowed.set(false);
        await fixture.whenStable();
        expect(fixture.nativeElement.querySelector('app-table')).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('oauth.permissionDenied');
    });
});
