import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { By } from '@angular/platform-browser';
import { Router, provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { SETTINGS_ROUTES } from '../settings.routes';
import { TranslocoService, TranslocoTestingModule } from '@jsverse/transloco';
import { Select } from '@openng/optimus-ui/select';
import { vi } from 'vitest';

import { UserPreferencesService } from '@services/user-preferences.service';
import { UserProfileService } from '@services/user-profile.service';
import { UserSettings } from './user-settings';

describe('UserSettings', () => {
    let fixture: ComponentFixture<UserSettings>;

    let originalLang: string;
    let originalDir: string;

    beforeEach(async () => {
        originalLang = document.documentElement.lang;
        originalDir = document.documentElement.dir;
        await TestBed.configureTestingModule({
            imports: [
                UserSettings,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: { account: 'Account' },
                            common: { actions: { copy: 'Copy' } },
                            preferences: {
                                language: {
                                    heading: 'Language',
                                    description: 'Choose dashboard language.',
                                    defaultLabel: 'Default language',
                                    defaultPlaceholder: 'Select language',
                                    searchPlaceholder: 'Search by language or ISO code',
                                    defaultHint: 'Used across the dashboard.',
                                    rtlBadge: 'RTL'
                                },
                                actions: { saveChanges: 'Save changes' },
                                status: { saved: 'Preferences saved', saveFailed: 'Could not save preferences' }
                            },
                            settings: {
                                security: { title: 'Security' },
                                user: {
                                    profile: {
                                        title: 'Profile',
                                        description: 'Update your personal details.',
                                        emailLabel: 'Email address',
                                        emailPlaceholder: 'you@example.com',
                                        givenNameLabel: 'Given name',
                                        givenNamePlaceholder: 'Ada',
                                        lastNameLabel: 'Last name',
                                        lastNamePlaceholder: 'Lovelace',
                                        userIdLabel: 'User ID',
                                        saveAction: 'Save profile',
                                        status: { saved: 'Profile saved' },
                                        validation: {
                                            emailRequired: 'Email is required.',
                                            emailInvalid: 'Please enter a valid email address.',
                                            emailTooLong: 'Email must be 320 characters or fewer.',
                                            nameTooLong: 'Names must be 120 characters or fewer.'
                                        },
                                        errors: {
                                            invalidInput: 'Please check the highlighted fields and try again.',
                                            emailTaken: 'That email is already in use.',
                                            notFound: 'Your account could not be found.',
                                            updateFailed: 'Failed to update profile. Please try again.'
                                        }
                                    },
                                    exportShortcut: {
                                        title: 'Analytics export',
                                        description: 'Export all accessible sites or individual site takeouts from the Import & Export hub.',
                                        action: 'Open Import & Export'
                                    }
                                }
                            }
                        },
                        de: {},
                        he: {},
                        pt: {}
                    },
                    translocoConfig: {
                        availableLangs: ['en', 'de', 'pt', 'he'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [
                provideRouter([{ path: 'settings', children: SETTINGS_ROUTES }]),
                provideHttpClient(),
                provideHttpClientTesting(),
                {
                    provide: UserProfileService,
                    useValue: {
                        profile: signal({
                            id: 'user-1',
                            email: 'demo@example.com',
                            given_name: 'Demo',
                            last_name: 'User',
                            display_name: 'Demo User',
                            avatar_url: ''
                        }),
                        isLoading: signal(false),
                        isSaving: signal(false),
                        updateProfile: vi.fn()
                    }
                }
            ]
        }).compileComponents();

        TestBed.inject(UserPreferencesService).applyPreferences({ default_locale: 'en', dismissed_onboarding_at: '2026-10-01' });
        fixture = TestBed.createComponent(UserSettings);
        await fixture.whenStable();
    });

    afterEach(() => {
        document.documentElement.lang = originalLang;
        document.documentElement.dir = originalDir;
    });

    it('redirects the historical preferences URL to Account', async () => {
        const harness = await RouterTestingHarness.create();
        await harness.navigateByUrl('/settings/preferences', UserSettings);
        expect(TestBed.inject(Router).url).toBe('/settings');
        expect(harness.routeNativeElement?.querySelector('#default-locale[role="combobox"]')).toBeTruthy();
    });

    it('saves a committed language once, restores the saved selection on failure and allows retry', async () => {
        const http = TestBed.inject(HttpTestingController);
        const transloco = TestBed.inject(TranslocoService);
        const preferences = TestBed.inject(UserPreferencesService);
        const combo = fixture.nativeElement.querySelector('#default-locale[role="combobox"]') as HTMLElement;
        const key = (name: string) => combo.dispatchEvent(new KeyboardEvent('keydown', { code: name, key: name, bubbles: true }));
        key('ArrowDown');
        await fixture.whenStable();
        expect(combo.getAttribute('aria-expanded')).toBe('true');
        key('ArrowDown');
        key('Escape');
        await fixture.whenStable();
        http.expectNone('/api/user/preferences');

        const chooseGerman = async () => {
            combo.click();
            await fixture.whenStable();
            const filter = document.querySelector<HTMLInputElement>('input[aria-label="Search by language or ISO code"]')!;
            filter.value = 'Deutsch';
            filter.dispatchEvent(new Event('input', { bubbles: true }));
            await fixture.whenStable();
            expect(document.querySelectorAll('[role="option"]')).toHaveLength(1);
            const option = document.querySelector<HTMLElement>('[role="option"][aria-label="German"]');
            expect(option).toBeTruthy();
            option!.click();
            await fixture.whenStable();
            const put = http.expectOne({ method: 'PUT', url: '/api/user/preferences' });
            expect(put.request.body).toEqual({ default_locale: 'de' });
            return put;
        };

        const first = await chooseGerman();
        expect(transloco.getActiveLang()).toBe('en');
        expect(preferences.isSaving()).toBe(true);
        expect(combo.getAttribute('aria-disabled')).toBe('true');
        first.flush({}, { status: 503, statusText: 'Unavailable' });
        await fixture.whenStable();
        expect(combo.textContent).toContain('English');
        expect(fixture.nativeElement.textContent).toContain('Could not save preferences');
        expect(preferences.preferences()?.default_locale).toBe('en');

        const retry = await chooseGerman();
        retry.flush({ default_locale: 'de' });
        await fixture.whenStable();
        expect(transloco.getActiveLang()).toBe('de');
        expect(combo.textContent).toContain('Deutsch');
        expect(preferences.preferences()?.dismissed_onboarding_at).toBe('2026-10-01');
        http.expectNone('/api/user/preferences');
    });

    it('shows the saved selection even when Transloco is displaying its fallback catalog', async () => {
        TestBed.inject(UserPreferencesService).applyPreferences({ default_locale: 'de' });
        TestBed.inject(TranslocoService).setActiveLang('en');
        await fixture.whenStable();
        expect(fixture.nativeElement.querySelector('#default-locale').textContent).toContain('German');
        TestBed.inject(HttpTestingController).expectNone('/api/user/preferences');
    });

    it('applies RTL after a successful selection from a configured RTL catalog', async () => {
        const combo = fixture.nativeElement.querySelector('#default-locale[role="combobox"]') as HTMLElement;
        combo.click();
        await fixture.whenStable();
        document.querySelector<HTMLElement>('[role="option"][aria-label="Hebrew"]')!.click();
        await fixture.whenStable();
        const put = TestBed.inject(HttpTestingController).expectOne({ method: 'PUT', url: '/api/user/preferences' });
        expect(put.request.body).toEqual({ default_locale: 'he' });
        expect(document.documentElement.dir).toBe('ltr');
        put.flush({ default_locale: 'he' });
        await fixture.whenStable();
        expect(document.documentElement.lang).toBe('he');
        expect(document.documentElement.dir).toBe('rtl');
    });

    it('replaces the user export tab with an account shortcut to the Import & Export hub', () => {
        const text = fixture.nativeElement.textContent;
        const links = Array.from(fixture.nativeElement.querySelectorAll('a')) as HTMLAnchorElement[];

        expect(text).toContain('Analytics export');
        expect(text).toContain('Export all accessible sites or individual site takeouts from the Import & Export hub.');
        expect(links.some((link) => link.getAttribute('href') === '/import-export/export')).toBe(true);
        expect(fixture.nativeElement.querySelector('[role="tab"][value="export"]')).toBeNull();
        expect(fixture.nativeElement.querySelector('p-splitbutton')).toBeNull();
    });

    it('makes the profile language selector searchable by language names and ISO code', () => {
        const select = fixture.debugElement.query(By.directive(Select)).componentInstance as Select;
        const options = select.options as { label: string; nativeLabel: string; value: string; isoCode: string; searchText: string }[];
        const german = options.find((option) => option.value === 'de');

        expect(select.filter).toBe(true);
        expect(select.filterBy).toBe('label,nativeLabel,value,isoCode,searchText');
        expect(select.filterPlaceholder).toBe('Search by language or ISO code');
        expect(select.ariaFilterLabel).toBe('Search by language or ISO code');
        expect(select.resetFilterOnHide).toBe(true);
        expect(select.appendTo()).toBe('body');
        expect(german?.label).toBe('German');
        expect(german?.nativeLabel).toBe('Deutsch');
        expect(german?.value).toBe('de');
        expect(german?.isoCode).toBe('DE');
        expect(german?.searchText).toContain('German');
        expect(german?.searchText).toContain('Deutsch');
        expect(german?.searchText).toContain('de');
        expect(german?.searchText).toContain('DE');
    });
});
