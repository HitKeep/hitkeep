import { inject, signal, computed, Service } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { DOCUMENT } from '@angular/common';
import { catchError, finalize, tap, throwError } from 'rxjs';
import { AuthService } from '@services/auth.service';
import { browserAppUrl } from '@core/interceptors/base-path.interceptor';

export interface UserProfile {
    id: string;
    email: string;
    given_name?: string;
    last_name?: string;
    display_name: string;
    avatar_url: string;
}

export interface AccountDeletionChoices {
    successors: { team_id: string; user_id: string }[];
    close_team_ids: string[];
}

export interface AccountDeletionPreparation {
    attempt_id: string;
    expires_at: string;
    purpose: 'delete-current-account';
    consequence_hash: string;
    phase: 'blocked' | 'prepared' | 'authorized' | 'removing' | 'complete';
    blockers: { code: string; team_id?: string }[];
    teams: {
        id: string;
        name: string;
        action: 'leave' | 'reparent' | 'close';
        closure_allowed: boolean;
        member_count: number;
        sites: { id: string; domain: string; owned: boolean }[];
        eligible_successors: { id: string; email: string }[];
        successor_id?: string;
        cancellation_required: boolean;
        billing_fingerprint?: string;
    }[];
    reauth: { return_path: string; methods: string[] | null; verified: boolean };
    retention: { billing_records: boolean; cancellation: 'period_end' };
}

export interface AccountDeletionResult {
    status: 'deleted';
    provider_revocations: { provider: 'apple'; status: 'revoked' | 'manual_revocation_required' }[];
}

@Service()
export class UserProfileService {
    private http = inject(HttpClient);
    private auth = inject(AuthService);
    private document = inject(DOCUMENT);

    readonly profile = signal<UserProfile | null>(null);
    readonly isLoading = signal(false);
    readonly isSaving = signal(false);
    readonly displayName = computed(() => {
        const profile = this.profile();
        if (!profile) return 'User';
        return profile.display_name || profile.email.split('@')[0] || 'User';
    });
    readonly avatarUrl = computed(() => browserAppUrl(this.document, this.profile()?.avatar_url || ''));

    loadProfile() {
        this.isLoading.set(true);
        return this.http.get<UserProfile>('/api/user/profile').pipe(
            tap((profile) => this.applyProfile(profile)),
            catchError((error) => {
                if (error?.status === 401) {
                    this.auth.markUnauthenticated();
                }
                return throwError(() => error);
            }),
            finalize(() => {
                this.isLoading.set(false);
            })
        );
    }

    updateProfile(payload: { email: string; given_name: string; last_name: string }) {
        this.isSaving.set(true);
        return this.http.put<UserProfile>('/api/user/profile', payload).pipe(
            tap((profile) => this.applyProfile(profile)),
            finalize(() => {
                this.isSaving.set(false);
            })
        );
    }

    prepareAccountDeletion(choices: AccountDeletionChoices) {
        return this.http.post<AccountDeletionPreparation>('/api/user/account/deletion/prepare', choices);
    }

    accountDeletionStatus(attemptId: string) {
        return this.http.get<AccountDeletionPreparation>(`/api/user/account/deletion/${encodeURIComponent(attemptId)}`);
    }

    deleteAccount(preparation: AccountDeletionPreparation) {
        return this.http
            .delete<AccountDeletionResult>('/api/user/account', {
                body: { attempt_id: preparation.attempt_id, consequence_hash: preparation.consequence_hash, confirm: true }
            })
            .pipe(
                tap((result) => {
                    this.profile.set(null);
                    this.auth.markUnauthenticated();
                    const view = this.document.defaultView;
                    if (view) {
                        try {
                            for (const storage of [view.localStorage, view.sessionStorage]) {
                                for (const key of Object.keys(storage)) {
                                    if (key.startsWith('hk_') || key.startsWith('hitkeep.')) storage.removeItem(key);
                                }
                            }
                        } catch {
                            /* Storage can be disabled; navigation still clears memory. */
                        }
                        const manualApple = result.provider_revocations.some((outcome) => outcome.provider === 'apple' && outcome.status === 'manual_revocation_required');
                        view.location.assign(browserAppUrl(this.document, '/account-deleted' + (manualApple ? '?manual_revocation=apple' : '')));
                    }
                })
            );
    }

    applyProfile(profile: UserProfile) {
        this.profile.set(profile);
        this.auth.markAuthenticated();
    }
}
