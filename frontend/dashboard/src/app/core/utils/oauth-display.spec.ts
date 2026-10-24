import { formatOAuthTime, oauthScopeLabelKey } from './oauth-display';

describe('oauthScopeLabelKey', () => {
    it('describes the four documented scopes without treating unknown values as permissions', () => {
        expect(oauthScopeLabelKey('sites:read')).toBe('oauth.scope.sitesRead');
        expect(oauthScopeLabelKey('analytics:read')).toBe('oauth.scope.analyticsRead');
        expect(oauthScopeLabelKey('tracking_domains:read')).toBe('oauth.scope.trackingDomainsRead');
        expect(oauthScopeLabelKey('ingest:write')).toBe('oauth.scope.ingestWrite');
        expect(oauthScopeLabelKey('admin:*')).toBeNull();
        expect(oauthScopeLabelKey('__proto__')).toBeNull();
    });
});

describe('formatOAuthTime', () => {
    it('uses the configured dashboard region and rejects malformed dates', () => {
        expect(formatOAuthTime('not-a-date', 'en')).toBeNull();
        const value = '2026-01-01T12:00:00Z';
        expect(formatOAuthTime(value, 'pt')).toBe(new Intl.DateTimeFormat('pt-BR', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)));
        expect(formatOAuthTime(value, 'de')).toBe(new Intl.DateTimeFormat('de-DE', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)));
    });
});
