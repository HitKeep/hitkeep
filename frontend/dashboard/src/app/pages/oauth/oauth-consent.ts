import { ChangeDetectionStrategy, Component, DestroyRef, computed, inject, signal } from '@angular/core';
import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { CheckboxModule } from '@openng/optimus-ui/checkbox';
import { MessageModule } from '@openng/optimus-ui/message';
import { SelectModule } from '@openng/optimus-ui/select';
import { finalize } from 'rxjs';

import { OAuthConsent } from '@models/oauth.types';
import { OAuthService } from '@services/oauth.service';
import { formatOAuthTime, oauthScopeLabelKey } from '@core/utils/oauth-display';
import { injectActiveLang } from '@core/i18n/active-lang';
import { browserAppUrl } from '@core/interceptors/base-path.interceptor';
import { AuthCard } from '@core/components/auth-card/auth-card';
import { Brand } from '@components/brand/brand';
import { UserControls } from '@components/user-controls/user-controls';
import { UserProfileService } from '@services/user-profile.service';

@Component({
    selector: 'app-oauth-consent',
    imports: [ReactiveFormsModule, RouterLink, TranslocoPipe, ButtonModule, CheckboxModule, MessageModule, SelectModule, AuthCard, Brand, UserControls],
    templateUrl: './oauth-consent.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class OAuthConsentPage {
    protected readonly scopeLabelKey = oauthScopeLabelKey;
    protected readonly action = browserAppUrl(inject(DOCUMENT), '/api/oauth/consent');
    private readonly language = injectActiveLang();
    protected readonly profile = inject(UserProfileService);
    protected readonly decisionErrorKey =
        new Map([
            ['expired', 'oauth.expired'],
            ['forbidden', 'oauth.permissionDenied'],
            ['unavailable', 'oauth.decisionUnavailable'],
            ['failed', 'oauth.decisionFailed']
        ]).get(inject(ActivatedRoute).snapshot.queryParamMap.get('error') ?? '') ?? null;
    private readonly oauth = inject(OAuthService);
    private readonly destroyRef = inject(DestroyRef);
    protected readonly consent = signal<OAuthConsent | null>(null);
    protected readonly loading = signal(false);
    protected readonly errorKey = signal<string | null>(null);
    protected readonly submitting = signal(false);
    private readonly now = signal(Date.now());
    private readonly model = signal({
        team_id: new FormControl('', { nonNullable: true }),
        site_ids: new FormControl<string[]>([], { nonNullable: true })
    });
    protected readonly form = compatForm(this.model);
    protected readonly selectedTeam = computed(() => this.consent()?.teams.find((team) => team.id === this.form.team_id().value()));
    protected readonly selectedSiteIds = computed(() => {
        const eligible = new Set(this.selectedTeam()?.sites.map((site) => site.id));
        return [...new Set(this.form.site_ids().value())].filter((id) => eligible.has(id));
    });
    protected readonly expired = computed(() => {
        const consent = this.consent();
        return !!consent && !(Date.parse(consent.expires_at) > this.now());
    });
    protected readonly expiryLabel = computed(() => formatOAuthTime(this.consent()?.expires_at ?? '', this.language()) ?? '');
    protected readonly canApprove = computed(() => !!this.selectedTeam() && this.selectedSiteIds().length > 0 && !this.expired() && !this.submitting());

    constructor() {
        this.form
            .team_id()
            .control()
            .valueChanges.pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe(() => this.form.site_ids().control().setValue([]));
        const timer = setInterval(() => this.now.set(Date.now()), 1000);
        this.destroyRef.onDestroy(() => clearInterval(timer));
        this.load();
    }

    protected load(): void {
        if (this.loading()) return;
        this.loading.set(true);
        this.errorKey.set(null);
        this.consent.set(null);
        this.form.team_id().control().setValue('');
        this.oauth
            .consent()
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.loading.set(false))
            )
            .subscribe({
                next: (consent) => this.consent.set(consent),
                error: (error: { status?: number }) => this.errorKey.set(error.status === 410 || error.status === 400 ? 'oauth.expired' : error.status === 401 || error.status === 403 ? 'oauth.permissionDenied' : 'oauth.loadFailed')
            });
    }

    protected submit(event: SubmitEvent): void {
        const decision = (event.submitter as HTMLButtonElement | null)?.value;
        if (this.submitting() || this.expired() || !this.consent() || (decision !== 'approve' && decision !== 'deny') || (decision === 'approve' && !this.canApprove())) {
            event.preventDefault();
            return;
        }
        // Preserve the decision before native serialization even if the submitter is disabled.
        const field = (event.currentTarget as HTMLFormElement).elements.namedItem('decision') as HTMLInputElement;
        field.value = decision;
        this.form.team_id().control().disable({ emitEvent: false });
        this.form.site_ids().control().disable({ emitEvent: false });
        this.submitting.set(true);
    }
}
import { DOCUMENT } from '@angular/common';
