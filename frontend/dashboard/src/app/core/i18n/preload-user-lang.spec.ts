import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideTransloco, TranslocoService } from '@jsverse/transloco';
import { TranslocoHttpLoader } from '../../transloco-loader';
import { vi } from 'vitest';
import { UserPreferencesService } from '@services/user-preferences.service';
import { preloadUserLang } from './preload-user-lang';

describe('dashboard language activation', () => {
    const root = document.documentElement;
    let originalLang: string;
    let originalDir: string;
    let transloco: TranslocoService;
    let http: HttpTestingController;

    beforeEach(() => {
        originalLang = root.lang;
        originalDir = root.dir;
        TestBed.configureTestingModule({
            providers: [
                provideHttpClient(),
                provideHttpClientTesting(),
                provideTransloco({
                    config: { availableLangs: ['en', 'fr', 'pt', 'he'], defaultLang: 'en', fallbackLang: 'en', failedRetries: 0 },
                    loader: TranslocoHttpLoader
                })
            ]
        });
        http = TestBed.inject(HttpTestingController);
        transloco = TestBed.inject(TranslocoService);
        vi.spyOn(navigator, 'language', 'get').mockReturnValue('ar-EG');
    });

    afterEach(() => {
        http.verify();
        root.lang = originalLang;
        root.dir = originalDir;
        vi.restoreAllMocks();
    });

    it('loads the first supported browser preference and applies saved preferences over it', async () => {
        vi.spyOn(navigator, 'languages', 'get').mockReturnValue(['ar-EG', 'fr-FR']);
        const bootstrap = TestBed.runInInjectionContext(preloadUserLang);
        http.expectOne('/i18n/fr.json').flush({ signIn: 'Se connecter' });
        await bootstrap;
        expect(transloco.translate('signIn')).toBe('Se connecter');
        TestBed.tick();
        expect([transloco.getActiveLang(), root.lang, root.dir]).toEqual(['fr', 'fr-FR', 'ltr']);

        const preferences = TestBed.inject(UserPreferencesService);
        preferences.applyPreferences({ default_locale: 'he-IL' });
        TestBed.tick();
        expect([transloco.getActiveLang(), root.lang, root.dir]).toEqual(['he', 'he-IL', 'rtl']);
        preferences.applyPreferences({ default_locale: 'pt_BR' });
        TestBed.tick();
        expect([transloco.getActiveLang(), root.lang, root.dir]).toEqual(['pt', 'pt-BR', 'ltr']);
        preferences.applyPreferences({ default_locale: 'fr-Arab' });
        TestBed.tick();
        expect([transloco.getActiveLang(), root.lang, root.dir]).toEqual(['fr', 'fr', 'ltr']);
        preferences.applyPreferences({ default_locale: 'ar' });
        TestBed.tick();
        expect([transloco.getActiveLang(), root.lang, root.dir]).toEqual(['fr', 'fr-FR', 'ltr']);
    });

    it('uses the default catalog and its direction when all preferences are unsupported', async () => {
        vi.spyOn(navigator, 'languages', 'get').mockReturnValue(['ar-EG', 'fa']);
        const bootstrap = TestBed.runInInjectionContext(preloadUserLang);
        http.expectOne('/i18n/en.json').flush({});
        await bootstrap;
        TestBed.tick();
        expect([transloco.getActiveLang(), root.lang, root.dir]).toEqual(['en', 'en', 'ltr']);
    });

    it('finishes startup with matching metadata when a cold RTL catalog falls back to cached English', async () => {
        transloco.load('en').subscribe();
        http.expectOne('/i18n/en.json').flush({ signIn: 'Sign in' });
        vi.spyOn(navigator, 'languages', 'get').mockReturnValue(['he-IL']);
        const bootstrap = TestBed.runInInjectionContext(preloadUserLang);
        http.expectOne('/i18n/he.json').flush({}, { status: 503, statusText: 'Unavailable' });
        await bootstrap;
        TestBed.tick();
        expect([transloco.getActiveLang(), root.lang, root.dir]).toEqual(['en', 'en', 'ltr']);
        expect(transloco.translate('signIn')).toBe('Sign in');
    });
});
