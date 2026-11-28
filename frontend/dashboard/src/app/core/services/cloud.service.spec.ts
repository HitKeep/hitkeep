import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';

import { of } from 'rxjs';
import { vi } from 'vitest';
import { billingStateUntilExpiry, CloudService, CloudSignupVerificationResendResponse } from '@services/cloud.service';

describe('CloudService', () => {
    let service: CloudService;
    let httpMock: HttpTestingController;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [CloudService, provideHttpClient(), provideHttpClientTesting()]
        });
        service = TestBed.inject(CloudService);
        httpMock = TestBed.inject(HttpTestingController);
    });

    afterEach(() => {
        httpMock.verify();
    });

    it('requests a Stripe sync only when confirming a return', () => {
        service.getBillingState({ sync: true }).subscribe();
        httpMock.expectOne('/api/cloud/billing/state?sync=1').flush({});
        service.getBillingState().subscribe();
        httpMock.expectOne('/api/cloud/billing/state').flush({});
    });

    it('removes the annual offer at the exclusive cutoff while review remains open', () => {
        vi.useFakeTimers();
        try {
            const ends = new Date('2026-12-07T08:00:00Z');
            vi.setSystemTime(new Date(ends.getTime() - 1));
            const state = { team_id: 'team', plan_code: 'free', subscription_status: 'free', promotion: { code: 'HKBF26', percent_off: 30, ends_at: ends.toISOString(), prices: {} } };
            const codes: (string | undefined)[] = [];
            billingStateUntilExpiry(of(state)).subscribe((current) => codes.push(current.promotion?.code));
            expect(codes).toEqual(['HKBF26']);
            vi.advanceTimersByTime(1);
            expect(codes).toEqual(['HKBF26', undefined]);
            billingStateUntilExpiry(of(state)).subscribe((current) => expect(current.promotion).toBeUndefined());
        } finally {
            vi.useRealTimers();
        }
    });

    it('keeps a configured promotion live beyond the browser timeout limit', () => {
        vi.useFakeTimers();
        try {
            const start = new Date('2026-08-01T00:00:00Z');
            vi.setSystemTime(start);
            const end = new Date(start.getTime() + 40 * 86400000);
            let code: string | undefined;
            billingStateUntilExpiry(of({ team_id: 'team', plan_code: 'free', subscription_status: 'free', promotion: { code: 'SUMMER', percent_off: 17.5, ends_at: end.toISOString(), prices: {} } })).subscribe(
                (state) => (code = state.promotion?.code)
            );
            vi.advanceTimersByTime(30 * 86400000);
            expect(code).toBe('SUMMER');
            vi.advanceTimersByTime(10 * 86400000);
            expect(code).toBeUndefined();
        } finally {
            vi.useRealTimers();
        }
    });

    it('shares one team state and clears both subscribers after paid redemption', () => {
        const team = 'team';
        const state = { team_id: team, plan_code: 'business', subscription_status: 'active' };
        const dashboard: (string | undefined)[] = [],
            billing: (string | undefined)[] = [];
        const left = service.watchBillingState(team).subscribe((s) => dashboard.push(s.promotion?.code));
        const right = service.watchBillingState(team).subscribe((s) => billing.push(s.promotion?.code));
        httpMock.expectOne('/api/cloud/billing/state').flush({ ...state, promotion: { code: 'HKBF26', percent_off: 30, ends_at: new Date(Date.now() + 60000).toISOString(), prices: {} } });
        service.redeemPromotion(team, 'HKBF26').subscribe();
        const post = httpMock.expectOne('/api/cloud/billing/promotion');
        expect(post.request.body).toEqual({ team_id: team, code: 'HKBF26' });
        post.flush(null, { status: 204, statusText: 'No Content' });
        httpMock.expectOne('/api/cloud/billing/state').flush(state);
        expect(dashboard).toEqual(['HKBF26', undefined]);
        expect(billing).toEqual(dashboard);
        left.unsubscribe();
        right.unsubscribe();
    });
    it('requests a privacy-neutral signup verification resend', () => {
        const response: CloudSignupVerificationResendResponse = {
            status: 'accepted',
            retry_after_seconds: 300
        };
        let actual: CloudSignupVerificationResendResponse | undefined;

        service.resendSignupVerification('user@example.com').subscribe((value) => (actual = value));

        const req = httpMock.expectOne('/api/cloud/signup/resend-verification');
        expect(req.request.method).toBe('POST');
        expect(req.request.body).toEqual({ email: 'user@example.com' });
        req.flush(response);
        expect(actual).toEqual(response);
    });
});
