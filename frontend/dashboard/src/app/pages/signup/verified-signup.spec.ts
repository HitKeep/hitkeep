import { DOCUMENT } from '@angular/common';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, convertToParamMap } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { TeamService } from '@services/team.service';
import { CloudService } from '@services/cloud.service';
import { of, NEVER, throwError } from 'rxjs';
import { vi } from 'vitest';

import { VerifiedSignup } from '@pages/signup/verified-signup';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { OAuthService } from '@services/oauth.service';
import { CloudSignupTrackingService } from '@services/cloud-signup-tracking.service';

describe('VerifiedSignup', () => {
    let queryParams: Record<string, string>;
    const hosted = vi.fn(() => true);
    const cloud = {
        getPlans: () => of([{ code: 'business', prices: { annual: { amount_minor: 39000, currency: 'EUR' } } }]),
        watchBillingState: () => of({ team_id: 'team-a', plan_code: 'free' }),
        createBillingCheckoutSession: vi.fn(() => NEVER)
    };
    const tracking = { install: vi.fn(), trackEvent: vi.fn() };
    const router = { navigateByUrl: vi.fn<(url: string) => Promise<boolean>>(() => Promise.resolve(true)) };

    beforeEach(async () => {
        vi.clearAllMocks();
        hosted.mockReturnValue(true);
        queryParams = {};
        await TestBed.configureTestingModule({
            imports: [VerifiedSignup, TranslocoTestingModule.forRoot({ langs: { en: {} }, translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }, preloadLangs: true })],
            providers: [
                provideTranslocoLocale({ langToLocaleMapping: { en: 'en-US' } }),
                { provide: CloudService, useValue: cloud },
                { provide: TeamService, useValue: { activeTeam: () => ({ id: 'team-a', name: 'Acme' }) } },
                { provide: OAuthService, useFactory: (router: Router) => ({ resuming: () => false, continueAfterAuthentication: vi.fn((target: string) => void router.navigateByUrl(target)) }), deps: [Router] },
                { provide: DashboardBootstrapService, useValue: { cloudHosted: hosted } },
                { provide: CloudSignupTrackingService, useValue: tracking },
                { provide: Router, useValue: router },
                { provide: DOCUMENT, useValue: document },
                {
                    provide: ActivatedRoute,
                    useValue: {
                        snapshot: {
                            get queryParamMap() {
                                return convertToParamMap(queryParams);
                            }
                        }
                    }
                }
            ]
        })
            .overrideComponent(VerifiedSignup, { set: { imports: [], template: '<div></div>' } })
            .compileComponents();
    });

    function create(): ComponentFixture<VerifiedSignup> {
        const fixture = TestBed.createComponent(VerifiedSignup);
        fixture.detectChanges();
        return fixture;
    }

    it('opens checkout for the verified team, plan and annual catalog price in one click', async () => {
        queryParams = { plan: 'business', billing: 'annual' };
        const fixture = create();
        await fixture.whenStable();
        const component = fixture.componentInstance;
        expect(router.navigateByUrl).not.toHaveBeenCalled();
        component['startCheckout']();
        expect(component['priceLabel']()).toBe('€390.00');
        expect(cloud.createBillingCheckoutSession).toHaveBeenCalledWith({ team_id: 'team-a', plan_code: 'business', billing: 'annual', locale: 'en' });
        component['startCheckout']();
        expect(cloud.createBillingCheckoutSession).toHaveBeenCalledTimes(1);
        expect(tracking.trackEvent).toHaveBeenCalledWith('signup_verified', expect.objectContaining({ plan: 'business', interval: 'annual' }));
    });

    it('exposes the billing recovery action if checkout fails', async () => {
        queryParams = { plan: 'business', billing: 'annual' };
        cloud.createBillingCheckoutSession.mockReturnValueOnce(throwError(() => new Error('unavailable')));
        const fixture = create();
        await fixture.whenStable();
        fixture.componentInstance['startCheckout']();
        expect(fixture.componentInstance['checkoutFailed']()).toBe(true);
        expect(fixture.componentInstance['checkoutPending']()).toBe(false);
    });

    it('does not offer Cloud payment on a self-hosted instance', () => {
        hosted.mockReturnValue(false);
        const component = create().componentInstance;
        component['startCheckout']();
        expect(router.navigateByUrl).toHaveBeenCalledWith('/dashboard');
        expect(tracking.install).not.toHaveBeenCalled();
    });

    it('lets a verified user continue on Free', () => {
        queryParams = { plan: 'pro', billing: 'monthly' };
        create().componentInstance['continueFree']();
        expect(router.navigateByUrl).toHaveBeenCalledWith('/dashboard');
    });
});
