import { AccountDeletion } from './account-deletion/account-deletion';
import { NgOptimizedImage } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';

import { ChangeDetectionStrategy, Component, computed, effect, inject, signal } from '@angular/core';
import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { CopyControl } from '@components/copy-control/copy-control';
import { PageFrame } from '@components/page-frame/page-frame';
import { localeFlagUrl } from '@core/i18n/flag-utils';
import { getBaseLanguage, getLocaleDirection, normalizeLocaleTag, TextDirection } from '@core/i18n/locale-utils';
import { SettingsCard } from '@features/settings/components/settings-card';
import { SettingsSecurity } from '@features/settings/components/settings-security';
import { UserPreferencesService } from '@services/user-preferences.service';
import { UserProfile, UserProfileService } from '@services/user-profile.service';
import { ButtonModule } from '@openng/optimus-ui/button';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { SelectModule } from '@openng/optimus-ui/select';
import { TabsModule } from '@openng/optimus-ui/tabs';

interface LanguageOption {
    label: string;
    nativeLabel: string;
    value: string;
    isoCode: string;
    searchText: string;
    flagUrl: string;
    direction: TextDirection;
}

interface EditableProfile {
    email: string;
    given_name: string;
    last_name: string;
}

type AvailableLang = string | { id: string; label: string };
type UserSettingsTab = 'account' | 'security';

@Component({
    selector: 'app-user-settings',
    imports: [AccountDeletion, ReactiveFormsModule, RouterLink, ButtonModule, InputTextModule, SelectModule, TabsModule, SettingsCard, SettingsSecurity, PageFrame, CopyControl, TranslocoPipe, NgOptimizedImage],
    templateUrl: './user-settings.html',
    styleUrl: './user-settings.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class UserSettings {
    private preferencesService = inject(UserPreferencesService);
    private profileService = inject(UserProfileService);
    private transloco = inject(TranslocoService);
    private route = inject(ActivatedRoute);
    private activeLanguage = this.transloco.activeLang;

    private readonly profileFormModel = signal({
        email: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.email, Validators.maxLength(320)] }),
        givenName: new FormControl('', { nonNullable: true, validators: [Validators.maxLength(120)] }),
        lastName: new FormControl('', { nonNullable: true, validators: [Validators.maxLength(120)] })
    });
    protected readonly profileForm = compatForm(this.profileFormModel);

    protected readonly language = new FormControl('', { nonNullable: true });
    protected readonly isProfileLoading = this.profileService.isLoading;
    protected readonly isProfileSaving = this.profileService.isSaving;
    protected readonly profileLoadError = signal<string | null>(null);
    protected readonly profileSaveError = signal<string | null>(null);
    protected readonly profileSaveState = signal<'idle' | 'saved' | 'error'>('idle');
    protected readonly initialProfile = signal<EditableProfile | null>(null);
    protected readonly profile = this.profileService.profile;

    protected readonly isLoading = this.preferencesService.isLoading;
    protected readonly isSaving = this.preferencesService.isSaving;
    protected readonly saveState = signal<'idle' | 'saved' | 'error'>('idle');
    protected readonly activeTab = signal<UserSettingsTab>('account');

    protected readonly languageOptions = computed(() => {
        this.activeLanguage();
        const options: LanguageOption[] = [];
        const seen = new Set<string>();
        const available = this.transloco.getAvailableLangs() as AvailableLang[];

        for (const entry of available) {
            const raw = typeof entry === 'string' ? entry : entry.id;
            const normalized = normalizeLocaleTag(raw);
            const base = getBaseLanguage(normalized) || normalized;
            if (!base || seen.has(base)) {
                continue;
            }
            seen.add(base);

            const label = this.optionLabel(base);
            const nativeLabel = this.languageName(base, base);
            const isoCode = base.toUpperCase();
            options.push({
                value: base,
                isoCode,
                label,
                nativeLabel,
                searchText: `${label} ${nativeLabel} ${base} ${isoCode}`,
                flagUrl: localeFlagUrl(base),
                direction: getLocaleDirection(base)
            });
        }

        return options.sort((a, b) => a.label.localeCompare(b.label, 'en', { sensitivity: 'base' }));
    });
    private readonly savedLanguage = computed(() => {
        const saved = getBaseLanguage(this.preferencesService.preferences()?.default_locale ?? '');
        return this.languageOptions().some((option) => option.value === saved) ? saved : this.activeLanguage();
    });

    protected readonly canSaveProfile = computed(() => {
        if (this.isProfileLoading() || this.isProfileSaving()) return false;
        if (this.profileForm().invalid()) return false;
        const initial = this.initialProfile();
        if (!initial) return false;
        return !this.profilesEqual(initial, this.currentProfile());
    });

    constructor() {
        if (this.route.snapshot.queryParamMap.get('tab') === 'security') {
            this.activeTab.set('security');
        }
        effect(() => {
            const shouldDisable = this.isProfileLoading() || this.isProfileSaving();
            const controls = [this.profileForm.email().control(), this.profileForm.givenName().control(), this.profileForm.lastName().control()];
            for (const control of controls) {
                if (shouldDisable && control.enabled) {
                    control.disable({ emitEvent: false });
                } else if (!shouldDisable && control.disabled) {
                    control.enable({ emitEvent: false });
                }
            }
        });

        effect(() => {
            const profile = this.profileService.profile();
            if (!profile) return;
            const editable = this.editableProfile(profile);
            this.profileForm.email().control().setValue(editable.email, { emitEvent: false });
            this.profileForm.givenName().control().setValue(editable.given_name, { emitEvent: false });
            this.profileForm.lastName().control().setValue(editable.last_name, { emitEvent: false });
            this.initialProfile.set(editable);
            this.profileSaveState.set('idle');
            this.profileSaveError.set(null);
            this.profileLoadError.set(null);
        });

        effect(() => {
            if (this.isSaving() || this.isLoading() || !this.preferencesService.preferences()) {
                this.language.disable({ emitEvent: false });
            } else {
                this.language.setValue(this.savedLanguage(), { emitEvent: false });
                this.language.enable({ emitEvent: false });
            }
        });
    }

    protected saveProfile(): void {
        if (this.profileForm().invalid()) {
            this.markProfileFieldsTouched();
            return;
        }
        if (!this.canSaveProfile()) {
            return;
        }

        this.profileSaveState.set('idle');
        this.profileSaveError.set(null);
        const payload = this.currentProfile();

        this.profileService.updateProfile(payload).subscribe({
            next: (profile) => {
                this.initialProfile.set(this.editableProfile(profile));
                this.profileSaveState.set('saved');
            },
            error: (error) => {
                this.profileSaveState.set('error');
                this.profileSaveError.set(this.profileErrorKey(error));
            }
        });
    }

    protected onProfileFieldChange(): void {
        if (this.profileSaveState() !== 'idle') {
            this.profileSaveState.set('idle');
        }
        if (this.profileSaveError()) {
            this.profileSaveError.set(null);
        }
    }

    protected saveLanguage(language: string): void {
        if (this.isSaving() || this.language.disabled || language === this.savedLanguage()) return;
        this.saveState.set('idle');
        this.preferencesService.save({ default_locale: language }).subscribe({
            next: () => this.saveState.set('saved'),
            error: () => {
                this.language.setValue(this.savedLanguage(), { emitEvent: false });
                this.saveState.set('error');
            }
        });
    }

    protected optionLabel(locale: string): string {
        const uiLanguage = getBaseLanguage(this.activeLanguage()) || 'en';
        return this.languageName(locale, uiLanguage);
    }

    private languageName(locale: string, displayLocale: string): string {
        const normalized = normalizeLocaleTag(locale);
        if (!normalized) return locale;
        const languageNames = this.displayNames('language', displayLocale);
        const language = normalized.split('-')[0];
        const languageName = languageNames?.of(language) ?? language;
        return languageName;
    }

    private currentProfile(): EditableProfile {
        return {
            email: this.profileForm.email().value().trim().toLowerCase(),
            given_name: this.profileForm.givenName().value().trim(),
            last_name: this.profileForm.lastName().value().trim()
        };
    }

    private editableProfile(profile: UserProfile): EditableProfile {
        return {
            email: profile.email,
            given_name: (profile.given_name ?? '').trim(),
            last_name: (profile.last_name ?? '').trim()
        };
    }

    private profilesEqual(a: EditableProfile, b: EditableProfile): boolean {
        return a.email === b.email && a.given_name === b.given_name && a.last_name === b.last_name;
    }

    private profileErrorKey(error: unknown): string {
        if (!(error instanceof HttpErrorResponse)) {
            return 'settings.user.profile.errors.updateFailed';
        }

        if (error.status === 409) {
            return 'settings.user.profile.errors.emailTaken';
        }
        if (error.status === 404) {
            return 'settings.user.profile.errors.notFound';
        }
        if (error.status === 400) {
            return 'settings.user.profile.errors.invalidInput';
        }

        return 'settings.user.profile.errors.updateFailed';
    }

    private markProfileFieldsTouched(): void {
        this.profileForm.email().markAsTouched();
        this.profileForm.givenName().markAsTouched();
        this.profileForm.lastName().markAsTouched();
    }

    private displayNames(type: 'language' | 'region' | 'script', locale: string): Intl.DisplayNames | null {
        if (typeof Intl === 'undefined' || !('DisplayNames' in Intl)) {
            return null;
        }
        try {
            return new Intl.DisplayNames([locale], { type });
        } catch {
            return null;
        }
    }
}
