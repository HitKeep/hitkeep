import { inject } from '@angular/core';
import { ActivatedRouteSnapshot } from '@angular/router';
import { TranslocoService } from '@jsverse/transloco';
import { map } from 'rxjs';
import { DashboardLanguageService } from '@core/i18n/dashboard-language.service';
import { normalizeLocaleTag } from '@core/i18n/locale-utils';
import { DASHBOARD_LANGUAGE_CODES } from '@core/i18n/supported-locales';

export function signupLanguage(selector: string | null) {
    const normalized = normalizeLocaleTag(selector ?? '');
    return DASHBOARD_LANGUAGE_CODES.find((code) => code === normalized);
}

// Both signup forms resolve their catalog after the signed-out guard and before mounting.
export function resolveSignupLanguage(route: ActivatedRouteSnapshot) {
    const requested = signupLanguage(route.queryParamMap.get('lang'));
    if (!requested) return null;
    const language = inject(DashboardLanguageService).activate(requested);
    return inject(TranslocoService)
        .load(language)
        .pipe(map(() => language));
}
