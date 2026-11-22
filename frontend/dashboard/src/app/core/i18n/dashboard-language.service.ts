import { DOCUMENT, effect, inject, Service, signal } from '@angular/core';
import { TranslocoService } from '@jsverse/transloco';
import { getBaseLanguage, getLocaleDirection, normalizeLocaleTag } from './locale-utils';

@Service()
export class DashboardLanguageService {
    private readonly transloco = inject(TranslocoService);
    private readonly document = inject(DOCUMENT);
    private readonly regionalLocale = signal('');

    constructor() {
        effect(() => {
            const language = this.transloco.activeLang();
            const locale = this.regionalLocale();
            this.document.documentElement.lang = getBaseLanguage(locale) === language ? locale : language;
            this.document.documentElement.dir = getLocaleDirection(language);
        });
    }

    activate(preferredLocale = ''): string {
        const available = this.transloco.getAvailableLangs().map((entry) => (typeof entry === 'string' ? entry : entry.id));
        const browser = typeof navigator === 'undefined' ? [] : [...(navigator.languages ?? []), navigator.language];
        let language = this.transloco.getDefaultLang();
        let region = '';
        for (const candidate of [preferredLocale, ...browser]) {
            const locale = normalizeLocaleTag(candidate || '');
            const match = available.includes(locale) ? locale : getBaseLanguage(locale);
            if (available.includes(match)) {
                language = match;
                // Only a regional suffix describes the same catalog without changing its script.
                region = /^[a-z]+-(?:[A-Z]{2}|\d{3})$/.test(locale) ? locale : '';
                break;
            }
        }
        this.regionalLocale.set(region);
        if (this.transloco.getActiveLang() !== language) this.transloco.setActiveLang(language);
        return language;
    }
}
