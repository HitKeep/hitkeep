import { Component } from '@angular/core';
import { DOCUMENT } from '@angular/common';
import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Router, provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { AuthService } from '@services/auth.service';
import { AnalyticsService } from '@services/analytics.service';
import { UserPreferencesService } from '@services/user-preferences.service';
import { Login } from './login';

@Component({ template: '' })
class ReauthDestination {}

describe('Login verified-primary MFA continuation with the real router', () => {
    const challenge = '12345678-1234-1234-1234-123456789abc';
    let auth: { verifyMfaTotp: ReturnType<typeof vi.fn>; resumeMfa: ReturnType<typeof vi.fn> };

    beforeEach(async () => {
        auth = {
            verifyMfaTotp: vi.fn(() => of(void 0)),
            resumeMfa: vi.fn(() => of({ status: 'mfa_required', challenge_token: challenge, factors: ['totp'] }))
        };
        await TestBed.configureTestingModule({
            imports: [TranslocoTestingModule.forRoot({ langs: { en: {} }, translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }, preloadLangs: true })],
            providers: [
                provideRouter([
                    { path: 'login', component: Login },
                    { path: 'settings', component: ReauthDestination },
                    { path: 'auth/native/consent', component: ReauthDestination },
                    { path: '', component: ReauthDestination }
                ]),
                provideHttpClient(),
                provideHttpClientTesting(),
                {
                    provide: AuthService,
                    useValue: {
                        ...auth,
                        status: () => 'unknown',
                        getSSOAvailability: () => of({ enabled: false }),
                        getSocialProviders: () => of({ providers: [] }),
                        consumeSocialMfaHandoff: () => null
                    }
                },
                { provide: AnalyticsService, useValue: { getSystemStatus: () => of({ cloud: null }) } },
                { provide: UserPreferencesService, useValue: { load: () => of(void 0) } }
            ]
        }).compileComponents();
    });

    afterEach(() => {
        TestBed.inject(HttpTestingController).verify();
        vi.restoreAllMocks();
    });

    it.each([`/settings?deletion_attempt=${'a'.repeat(43)}`, `/auth/native/consent?request=${'b'.repeat(43)}`, `/auth/native/manage?request=${'c'.repeat(43)}`])('keeps %s after fragment cleanup and successful MFA', async (target) => {
        const harness = await RouterTestingHarness.create(`/login?returnUrl=${encodeURIComponent(target)}#mfa_challenge=${challenge}`);
        await harness.fixture.whenStable();
        const router = TestBed.inject(Router);
        expect(router.parseUrl(router.url).fragment).toBeNull();
        expect(router.parseUrl(router.url).queryParams['returnUrl']).toBe(target);
        expect(auth.resumeMfa).toHaveBeenCalledWith(challenge);

        const login = harness.routeDebugElement!.componentInstance as Login;
        const assign = vi.fn();
        vi.spyOn(TestBed.inject(DOCUMENT), 'defaultView', 'get').mockReturnValue({ location: { assign } } as unknown as Document['defaultView']);
        login['loginForm'].mfaCode().control().setValue('123456');
        login['verifyTotpMfa']();
        await harness.fixture.whenStable();

        expect(auth.verifyMfaTotp).toHaveBeenCalledWith(challenge, '123456');
        if (target.startsWith('/auth/native/')) {
            expect(assign).toHaveBeenCalledWith(target);
            expect(router.parseUrl(router.url).queryParams['returnUrl']).toBe(target);
        } else {
            expect(router.url).toBe(target);
            expect(assign).not.toHaveBeenCalled();
        }
        TestBed.inject(HttpTestingController).expectNone('/api/oauth/continuation');
    });
});
