const SCOPE_LABELS = new Map([
    ['sites:read', 'oauth.scope.sitesRead'],
    ['analytics:read', 'oauth.scope.analyticsRead'],
    ['tracking_domains:read', 'oauth.scope.trackingDomainsRead'],
    ['ingest:write', 'oauth.scope.ingestWrite']
]);

// Display copy only; the backend owns scope validation and authorization.
export function oauthScopeLabelKey(scope: string): string | null {
    return SCOPE_LABELS.get(scope) ?? null;
}
import { DASHBOARD_LANG_LOCALES, DashboardLanguage } from '@core/i18n/supported-locales';

export function formatOAuthTime(value: string, language: string): string | null {
    const timestamp = Date.parse(value);
    if (Number.isNaN(timestamp)) return null;
    return new Intl.DateTimeFormat(DASHBOARD_LANG_LOCALES[language as DashboardLanguage] ?? language, { dateStyle: 'medium', timeStyle: 'short' }).format(timestamp);
}
