import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Router, provideRouter } from '@angular/router';
import { vi } from 'vitest';
import { APPLICATION_ERROR_STATE_KEY } from '@services/application-error-navigation.service';

import { OAuthService } from './oauth.service';

describe('OAuthService', () => {
    let service: OAuthService;
    let http: HttpTestingController;

    beforeEach(() => {
        TestBed.configureTestingModule({ providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()] });
        service = TestBed.inject(OAuthService);
        http = TestBed.inject(HttpTestingController);
    });

    afterEach(() => http.verify());

    it('reads cookie-bound consent without OAuth parameters or credentials', () => {
        service.consent().subscribe();
        const request = http.expectOne('/api/oauth/consent');
        expect(request.request.method).toBe('GET');
        expect(request.request.body).toBeNull();
        expect(request.request.headers.has('Authorization')).toBe(false);
        request.flush({});
    });

    it('reads only the validated pending continuation flag', () => {
        let pending = false;
        service.continuation().subscribe((value) => (pending = value.pending));
        const request = http.expectOne('/api/oauth/continuation');
        request.flush({ pending: true });
        expect(pending).toBe(true);
    });

    it('resumes only validated pending consent after authentication', () => {
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
        service.continueAfterAuthentication('/dashboard?range=7d');
        http.expectOne('/api/oauth/continuation').flush({ pending: true });
        expect(navigate).toHaveBeenCalledWith('/oauth/consent');
        expect(service.resuming()).toBe(false);
    });

    it('preserves the existing destination without forcing pending consent on later setup navigation', () => {
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
        service.continueAfterAuthentication('/settings/billing?plan=pro');
        http.expectOne('/api/oauth/continuation').flush({ pending: false });
        expect(navigate).toHaveBeenCalledWith('/settings/billing?plan=pro');
    });

    it.each(['/signup/verified?plan=pro&billing=annual', '/admin/team?billing=annual&purchase=review&plan=pro'])('finishes the existing native account or billing step before consent at %s', (target) => {
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
        service.continueAfterAuthentication(target);
        http.expectNone('/api/oauth/continuation');
        expect(navigate).toHaveBeenCalledWith(target);
    });

    it('keeps continuation outages retryable without copying backend bodies into navigation', () => {
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
        service.continueAfterAuthentication('/dashboard');
        http.expectOne('/api/oauth/continuation').flush('private response', { status: 503, statusText: 'Unavailable' });
        const [url, extras] = navigate.mock.calls[0];
        expect(TestBed.inject(Router).serializeUrl(url as never)).toBe('/error');
        expect(extras?.state?.[APPLICATION_ERROR_STATE_KEY]).toEqual({ version: 1, kind: 'server', context: 'dashboard-bootstrap', returnUrl: '/oauth/consent', status: 503 });
        expect(service.resuming()).toBe(false);
    });

    it('reads the current users grants and encodes the revoked grant ID', () => {
        service.grants().subscribe();
        http.expectOne('/api/user/oauth-grants').flush([]);
        service.revoke('grant/other').subscribe();
        const request = http.expectOne('/api/user/oauth-grants/grant%2Fother');
        expect(request.request.method).toBe('DELETE');
        request.flush(null, { status: 204, statusText: 'No Content' });
    });
});
