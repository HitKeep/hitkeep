import { routes } from './app.routes';
import { capabilityGuard } from '@guards/capability-guard';
import { TEAM_CAPABILITIES } from '@core/access/capabilities';
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
    it('matches OAuth registration administration to the backend team capability', () => {
        const layout = routes.find((route) => route.path === '');
        const admin = layout?.children?.find((route) => route.path === 'integration/oauth-clients');
        expect(admin?.canActivate).toContain(capabilityGuard);
        expect(admin?.data?.['activeTeamCapability']).toBe(TEAM_CAPABILITIES.manageApiClients);
        expect(admin?.loadComponent).toBeDefined();
    });
});
