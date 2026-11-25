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

    it('removes the annual offer at the exclusive cutoff while review remains open', () => {
        vi.useFakeTimers();
        try {
            const ends = new Date('2026-12-07T08:00:00Z');
            vi.setSystemTime(new Date(ends.getTime() - 1));
            const state = { team_id: 'team', plan_code: 'free', subscription_status: 'free', annual_offer: { code: 'HKBF26', ends_at: ends.toISOString(), prices: {} } };
            const codes: (string | undefined)[] = [];
            billingStateUntilExpiry(of(state)).subscribe((current) => codes.push(current.annual_offer?.code));
            expect(codes).toEqual(['HKBF26']);
            vi.advanceTimersByTime(1);
            expect(codes).toEqual(['HKBF26', undefined]);
            billingStateUntilExpiry(of(state)).subscribe((current) => expect(current.annual_offer).toBeUndefined());
        } finally {
            vi.useRealTimers();
        }
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
