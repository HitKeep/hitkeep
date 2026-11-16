import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { NativeManagePage } from './native-manage';

const handle = 'a'.repeat(43);

describe('NativeManagePage', () => {
    let fixture: ComponentFixture<NativeManagePage>;
    beforeEach(() => {
        TestBed.configureTestingModule({
            imports: [NativeManagePage, TranslocoTestingModule.forRoot({ langs: { en: {} }, translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }, preloadLangs: true })],
            providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting(), { provide: ActivatedRoute, useValue: { snapshot: { queryParamMap: convertToParamMap({ request: handle }) } } }]
        });
    });
    afterEach(() => TestBed.inject(HttpTestingController).verify());

    async function load(phase = 'pending', action = 'social_link') {
        fixture = TestBed.createComponent(NativeManagePage);
        TestBed.inject(HttpTestingController)
            .expectOne(`/api/auth/native/consent?request=${handle}`)
            .flush({
                purpose: 'management',
                request: handle,
                client_id: 'hitkeep-ios',
                issuer: 'https://example.test/prefix',
                email: '<img src=x>',
                csrf_token: 'browser-csrf',
                expires_at: new Date(Date.now() + 60_000).toISOString(),
                management: { action, provider: 'google', team_id: '11111111-2222-3333-4444-555555555555', team_name: 'Exact team', phase }
            });
        await fixture.whenStable();
    }

    it('requires an explicit exact-action decision without offering session persistence', async () => {
        await load();
        expect(fixture.nativeElement.querySelector('dl img')).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('nativeAuth.management.social_link');
        expect(fixture.nativeElement.textContent).toContain('google');
        const form = fixture.nativeElement.querySelector('form') as HTMLFormElement;
        const approve = form.querySelector('button[value="approve"]') as HTMLButtonElement;
        const event = new SubmitEvent('submit', { submitter: approve, bubbles: true, cancelable: true });
        form.dispatchEvent(event);
        expect(event.defaultPrevented).toBe(false);
        expect([...new FormData(form).entries()]).toEqual([
            ['request', handle],
            ['csrf_token', 'browser-csrf'],
            ['decision', 'approve']
        ]);
        expect(form.querySelector('[name="remember_me"]')).toBeNull();
        const repeated = new SubmitEvent('submit', { submitter: approve, bubbles: true, cancelable: true });
        form.dispatchEvent(repeated);
        expect(repeated.defaultPrevented).toBe(true);
    });

    it('does not offer a receipt while the provider operation is incomplete', async () => {
        await load('started');
        expect(fixture.nativeElement.querySelector('button[value="approve"]')).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('nativeAuth.management.restart');
    });

    it('offers a browser POST receipt only for server-recorded success', async () => {
        await load('succeeded');
        expect(fixture.nativeElement.textContent).toContain('nativeAuth.management.completed');
        expect(fixture.nativeElement.querySelector('button[value="approve"]').textContent).toContain('nativeAuth.management.return');
    });

    it('starts only the server-frozen exact team using existing browser APIs', async () => {
        await load('authorized', 'search_console_connect');
        (fixture.nativeElement.querySelector('button[type="button"]') as HTMLButtonElement).click();
        const operation = TestBed.inject(HttpTestingController).expectOne('/api/user/teams/11111111-2222-3333-4444-555555555555/integrations/google-search-console/connect');
        expect(operation.request.method).toBe('POST');
        expect(operation.request.headers.get('X-Hitkeep-Native-Management')).toBe(handle);
        expect(operation.request.body).toEqual({});
        operation.flush({}, { status: 403, statusText: 'Forbidden' });
        await fixture.whenStable();
        expect(fixture.nativeElement.textContent).toContain('nativeAuth.management.restart');
    });
});
