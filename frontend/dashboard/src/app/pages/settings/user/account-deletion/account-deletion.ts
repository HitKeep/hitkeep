import { DOCUMENT } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { CheckboxModule } from '@openng/optimus-ui/checkbox';
import { SelectModule } from '@openng/optimus-ui/select';
import { finalize } from 'rxjs';

import { browserAppUrl } from '@core/interceptors/base-path.interceptor';
import { SettingsCard } from '@features/settings/components/settings-card';
import { AccountDeletionPreparation, UserProfileService } from '@services/user-profile.service';

@Component({
    selector: 'app-account-deletion',
    imports: [FormsModule, RouterLink, TranslocoPipe, ButtonModule, CheckboxModule, SelectModule, SettingsCard],
    templateUrl: './account-deletion.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AccountDeletion {
    private readonly profile = inject(UserProfileService);
    private readonly route = inject(ActivatedRoute);
    private readonly document = inject(DOCUMENT);
    protected readonly review = signal<AccountDeletionPreparation | null>(null);
    protected readonly busy = signal(false);
    protected readonly error = signal<string | null>(null);
    protected readonly confirmed = signal(false);
    protected readonly needsReview = signal(false);
    protected readonly successors = signal<Record<string, string>>({});
    protected readonly closedTeams = signal<string[]>([]);
    protected readonly reauthUrl = computed(() => browserAppUrl(this.document, this.review()?.reauth.return_path || '/settings'));
    protected readonly canDelete = computed(() => {
        const review = this.review();
        return !!review?.attempt_id && review.reauth.verified && review.blockers.length === 0 && this.confirmed() && !this.needsReview() && !this.busy();
    });

    constructor() {
        const attempt = this.route.snapshot.queryParamMap.get('deletion_attempt');
        if (attempt) {
            this.busy.set(true);
            this.profile
                .accountDeletionStatus(attempt)
                .pipe(finalize(() => this.busy.set(false)))
                .subscribe({
                    next: (review) => this.applyReview(review),
                    error: (error: HttpErrorResponse) => this.setError(error)
                });
        }
    }

    protected prepare() {
        if (this.busy()) return;
        this.busy.set(true);
        this.error.set(null);
        this.confirmed.set(false);
        this.profile
            .prepareAccountDeletion({
                successors: Object.entries(this.successors())
                    .filter(([, id]) => !!id)
                    .map(([team_id, user_id]) => ({ team_id, user_id })),
                close_team_ids: this.closedTeams()
            })
            .pipe(finalize(() => this.busy.set(false)))
            .subscribe({
                next: (review) => this.applyReview(review),
                error: (error: HttpErrorResponse) => this.setError(error)
            });
    }

    protected selectSuccessor(teamId: string, userId: string) {
        this.successors.update((values) => ({ ...values, [teamId]: userId }));
        this.needsReview.set(true);
        this.confirmed.set(false);
    }

    protected selectClosure(teamId: string, close: boolean) {
        this.closedTeams.update((ids) => (close ? [...ids.filter((id) => id !== teamId), teamId] : ids.filter((id) => id !== teamId)));
        this.needsReview.set(true);
        this.confirmed.set(false);
    }

    protected deleteAccount() {
        const review = this.review();
        if (!review || !this.canDelete()) return;
        this.busy.set(true);
        this.error.set(null);
        this.profile
            .deleteAccount(review)
            .pipe(finalize(() => this.busy.set(false)))
            .subscribe({
                error: (error: HttpErrorResponse) => this.setError(error)
            });
    }

    protected blockerKey(code: string) {
        switch (code) {
            case 'last_operator':
                return 'settings.user.deletion.lastOperator';
            case 'ownership_transfer_required':
                return 'settings.user.deletion.handoverRequired';
            case 'successor_required':
                return 'settings.user.deletion.successorRequired';
            case 'invalid_successor':
                return 'settings.user.deletion.invalidSuccessor';
            default:
                return 'settings.user.deletion.policyBlocked';
        }
    }

    protected teamName(teamId?: string) {
        return this.review()?.teams.find((team) => team.id === teamId)?.name || '';
    }

    private applyReview(review: AccountDeletionPreparation) {
        this.review.set(review);
        this.successors.set(Object.fromEntries(review.teams.filter((team) => team.successor_id).map((team) => [team.id, team.successor_id!])));
        this.closedTeams.set(review.teams.filter((team) => team.action === 'close').map((team) => team.id));
        this.needsReview.set(false);
        this.confirmed.set(false);
    }

    private setError(error: HttpErrorResponse) {
        const code = error.error?.error?.code;
        this.error.set(code === 'reauth_required' ? 'settings.user.deletion.reauthRequired' : code === 'consequences_changed' ? 'settings.user.deletion.consequencesChanged' : 'settings.user.deletion.unavailable');
        this.confirmed.set(false);
    }
}
