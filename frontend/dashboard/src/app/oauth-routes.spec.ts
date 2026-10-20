import { routes } from './app.routes';
import { authGuard } from '@guards/auth-guard';
import { SETTINGS_ROUTES } from '@pages/settings/settings.routes';

describe('OAuth route boundaries', () => {
    it('keeps native consent authenticated and outside the dashboard layout', () => {
        const consent = routes.find((route) => route.path === 'oauth/consent');
        expect(consent?.canActivate).toContain(authGuard);
        expect(consent?.loadComponent).toBeDefined();
        expect(consent?.data?.['titleKey']).toBe('oauth.consentTitle');
    });

    it('exposes application management only below the existing authenticated settings boundary', () => {
        const layout = routes.find((route) => route.path === '');
        expect(layout?.canActivate).toContain(authGuard);
        expect(layout?.children?.find((route) => route.path === 'settings')).toBeDefined();
        expect(SETTINGS_ROUTES.find((route) => route.path === 'connected-applications')?.loadComponent).toBeDefined();
    });
});
