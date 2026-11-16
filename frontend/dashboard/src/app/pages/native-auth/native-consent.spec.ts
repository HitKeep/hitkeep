import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { NativeConsentPage } from './native-consent';

const handle = 'a'.repeat(43);

describe('NativeConsentPage', () => {
    let fixture: ComponentFixture<NativeConsentPage>;
    beforeEach(() => {
        TestBed.configureTestingModule({
            imports: [NativeConsentPage, TranslocoTestingModule.forRoot({ langs: { en: {} }, translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }, preloadLangs: true })],
            providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting(), { provide: ActivatedRoute, useValue: { snapshot: { queryParamMap: convertToParamMap({ request: handle }) } } }]
        });
    });
    afterEach(() => TestBed.inject(HttpTestingController).verify());

    async function load(expiresAt = new Date(Date.now() + 60_000).toISOString()) {
        fixture = TestBed.createComponent(NativeConsentPage);
        TestBed.inject(HttpTestingController).expectOne(`/api/auth/native/consent?request=${handle}`).flush({
            request: handle,
            client_id: 'hitkeep-ios',
            issuer: 'https://example.test/prefix',
            email: '<img src=x onerror=alert(1)>',
            csrf_token: 'browser-csrf',
            remember_me: true,
            expires_at: expiresAt
        });
        await fixture.whenStable();
    }

    it('shows full-account consent and final identity, posting only the bound browser decision', async () => {
        await load();
        expect(fixture.nativeElement.textContent).toContain('nativeAuth.authority');
        expect(fixture.nativeElement.textContent).toContain('https://example.test/prefix');
        expect(fixture.nativeElement.querySelector('dl img')).toBeNull();
        const form = fixture.nativeElement.querySelector('form') as HTMLFormElement;
        const decision = form.querySelector('button[value="approve"]') as HTMLButtonElement;
        const event = new SubmitEvent('submit', { submitter: decision, bubbles: true, cancelable: true });
        form.dispatchEvent(event);
        expect(event.defaultPrevented).toBe(false);
        expect(form.method).toBe('post');
        expect(new URL(form.action).pathname).toContain('/api/auth/native/authorize');
        expect([...new FormData(form).entries()]).toEqual([
            ['request', handle],
            ['csrf_token', 'browser-csrf'],
            ['decision', 'approve'],
            ['remember_me', 'true']
        ]);
        const repeated = new SubmitEvent('submit', { submitter: decision, bubbles: true, cancelable: true });
        form.dispatchEvent(repeated);
        expect(repeated.defaultPrevented).toBe(true);
    });

    it('never submits an expired approval', async () => {
        await load(new Date(Date.now() - 1000).toISOString());
        const form = fixture.nativeElement.querySelector('form') as HTMLFormElement;
        const event = new SubmitEvent('submit', { submitter: form.querySelector('button[value="approve"]'), bubbles: true, cancelable: true });
        form.dispatchEvent(event);
        expect(event.defaultPrevented).toBe(true);
    });
});
