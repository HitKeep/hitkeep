import { DOCUMENT } from '@angular/common';
import { TestBed } from '@angular/core/testing';
import { ActivatedRouteSnapshot, CanActivateFn, provideRouter, RouterStateSnapshot } from '@angular/router';
import { routes } from '../../app.routes';

describe('private frame routing', () => {
    const ownWindow = {};
    const check = (url: string) => {
        const guard = routes.find((route) => route.path === '')?.canActivateChild?.[0] as CanActivateFn | undefined;
        expect(guard).toBeTypeOf('function');
        return TestBed.runInInjectionContext(() => guard!({} as ActivatedRouteSnapshot, { url } as RouterStateSnapshot));
    };

    beforeEach(() => {
        TestBed.configureTestingModule({ providers: [provideRouter([]), { provide: DOCUMENT, useValue: { defaultView: { self: ownWindow, top: {} } } }] });
    });

    it('blocks navigation from an embedded public shell to all private routes', () => {
        for (const url of ['/oauth/consent', '/login', '/signup', '/reset-password', '/dashboard', '/settings/profile', '/report-confirmation', '/share']) {
            expect(check(url), url).toBe(false);
        }
    });

    it('keeps public shared dashboards and QR reports embeddable', () => {
        for (const url of ['/share/token/dashboard', '/share/token/events?range=7', '/qr-share/token']) {
            expect(check(url), url).toBe(true);
        }
    });

    it('allows private routes in a top-level document', () => {
        TestBed.overrideProvider(DOCUMENT, { useValue: { defaultView: { self: ownWindow, top: ownWindow } } });
        expect(check('/oauth/consent')).toBe(true);
        expect(check('/dashboard')).toBe(true);
    });

    it('protects direct routes and repeated child navigation with the same boundary', () => {
        const childGuard = routes.find((route) => route.path === '')?.canActivateChild?.[0];
        expect(childGuard).toBeTypeOf('function');
        for (const route of routes) {
            expect(route.canActivate?.[0], route.path).toBe(childGuard);
            expect(route.canActivateChild?.[0], route.path).toBe(childGuard);
        }
    });
});
