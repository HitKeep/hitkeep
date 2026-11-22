import { inject, signal, Service } from '@angular/core';
import { HttpClient, HttpContext } from '@angular/common/http';
import { finalize, tap } from 'rxjs';
import { DashboardLanguageService } from '@core/i18n/dashboard-language.service';
import { SKIP_AUTH_REDIRECT } from '@core/interceptors/auth.interceptor';

export interface UserPreferences {
    default_locale: string;
    dismissed_onboarding_at?: string;
}

@Service()
export class UserPreferencesService {
    private http = inject(HttpClient);
    private language = inject(DashboardLanguageService);

    readonly preferences = signal<UserPreferences | null>(null);
    readonly isLoading = signal(false);
    readonly isSaving = signal(false);

    load(options?: { skipAuthRedirect?: boolean }) {
        this.isLoading.set(true);
        const context = options?.skipAuthRedirect ? new HttpContext().set(SKIP_AUTH_REDIRECT, true) : undefined;
        return this.http.get<UserPreferences>('/api/user/preferences', { context }).pipe(
            tap((prefs) => this.applyPreferences(prefs)),
            finalize(() => {
                this.isLoading.set(false);
            })
        );
    }

    save(preferences: UserPreferences) {
        this.isSaving.set(true);
        return this.http.put<UserPreferences>('/api/user/preferences', preferences).pipe(
            tap((prefs) => this.applyPreferences({ ...this.preferences(), ...prefs })),
            finalize(() => {
                this.isSaving.set(false);
            })
        );
    }

    applyPreferences(prefs: UserPreferences) {
        this.preferences.set(prefs);
        this.language.activate(prefs.default_locale);
    }
}
