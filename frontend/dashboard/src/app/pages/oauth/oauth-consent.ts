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
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { finalize } from 'rxjs';

import { OAuthConsent } from '@models/oauth.types';
import { OAuthService } from '@services/oauth.service';
import { formatOAuthTime, oauthScopeLabelKey } from '@core/utils/oauth-display';
import { injectActiveLang } from '@core/i18n/active-lang';
import { browserAppUrl } from '@core/interceptors/base-path.interceptor';
import { AuthCard } from '@core/components/auth-card/auth-card';
import { Brand } from '@components/brand/brand';
import { UserProfileService } from '@services/user-profile.service';
import { UserMenuService } from '@services/user-menu.service';

@Component({
    selector: 'app-oauth-consent',
    imports: [ReactiveFormsModule, RouterLink, TranslocoPipe, ButtonModule, CheckboxModule, MessageModule, SelectModule, InputTextModule, AuthCard, Brand],
    templateUrl: './oauth-consent.html',
    styleUrl: './oauth-consent.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class OAuthConsentPage {
    protected readonly scopeLabelKey = oauthScopeLabelKey;
    protected readonly action = browserAppUrl(inject(DOCUMENT), '/api/oauth/consent');
    private readonly language = injectActiveLang();
    protected readonly profile = inject(UserProfileService);
    protected readonly accountMenu = inject(UserMenuService);
    protected readonly claimReturned = inject(ActivatedRoute).snapshot.queryParamMap.get('claim') === 'complete';
    protected readonly decisionErrorKey =
        new Map([
            ['expired', 'oauth.expired'],
            ['forbidden', 'oauth.permissionDenied'],
            ['unavailable', 'oauth.decisionUnavailable'],
            ['failed', 'oauth.decisionFailed'],
            ['invalid_code', 'oauth.agentCodeInvalid']
        ]).get(inject(ActivatedRoute).snapshot.queryParamMap.get('error') ?? '') ?? null;
    private readonly oauth = inject(OAuthService);
    private readonly destroyRef = inject(DestroyRef);
    protected readonly consent = signal<OAuthConsent | null>(null);
    protected readonly loading = signal(false);
    protected readonly errorKey = signal<string | null>(null);
    protected readonly submitting = signal(false);
    protected readonly siteDomain = new FormControl('', { nonNullable: true });
    protected readonly creatingSite = signal(false);
    protected readonly createSiteErrorKey = signal<string | null>(null);
    private readonly now = signal(Date.now());
    private readonly model = signal({
        team_id: new FormControl('', { nonNullable: true }),
        claim_code: new FormControl('', { nonNullable: true }),
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
    protected readonly billingOnly = computed(() => {
        const scopes = this.consent()?.scopes ?? [];
        return scopes.length > 0 && scopes.every((scope) => scope === 'billing:write');
    });
    protected readonly canApprove = computed(
        () => !!this.selectedTeam() && (this.billingOnly() || this.selectedSiteIds().length > 0) && (!this.consent()?.agent_claim || /^[0-9]{6}$/.test(this.form.claim_code().value())) && !this.expired() && !this.submitting() && !this.creatingSite()
    );

    constructor() {
        this.form
            .team_id()
            .control()
            .valueChanges.pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe(() => {
                this.form.site_ids().control().setValue([]);
                this.createSiteErrorKey.set(null);
            });
        const timer = setInterval(() => this.now.set(Date.now()), 1000);
        this.destroyRef.onDestroy(() => clearInterval(timer));
        if (!this.claimReturned) this.load();
    }

    protected load(selectedTeamId?: string, selectedSiteId?: string): void {
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
                next: (consent) => {
                    this.consent.set(consent);
                    this.siteDomain.setValue(consent.suggested_domain ?? '');
                    const team = consent.teams.find((team) => team.id === selectedTeamId) ?? (!this.billingOnly() && consent.teams.length === 1 ? consent.teams[0] : undefined);
                    if (team) {
                        this.form.team_id().control().setValue(team.id);
                        if (selectedSiteId && team.sites.some((site) => site.id === selectedSiteId)) this.form.site_ids().control().setValue([selectedSiteId]);
                    }
                },
                error: (error: { status?: number }) => this.errorKey.set(error.status === 410 || error.status === 400 ? 'oauth.expired' : error.status === 401 || error.status === 403 ? 'oauth.permissionDenied' : 'oauth.loadFailed')
            });
    }

    protected createSite(): void {
        const team = this.selectedTeam();
        const consent = this.consent();
        const domain = this.siteDomain.value.trim();
        if (!team?.can_create_site || !consent || !domain || this.creatingSite() || this.submitting() || this.expired()) return;
        this.creatingSite.set(true);
        this.createSiteErrorKey.set(null);
        this.oauth
            .createSite(team.id, consent.csrf_token, domain)
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.creatingSite.set(false))
            )
            .subscribe({
                next: (site) => this.load(team.id, site.id),
                error: (error: { status?: number }) =>
                    this.createSiteErrorKey.set(
                        error.status === 400 ? 'oauth.siteCreateInvalid' : error.status === 403 ? 'oauth.permissionDenied' : error.status === 409 ? 'oauth.siteCreateExists' : error.status === 410 ? 'oauth.expired' : 'oauth.siteCreateFailed'
                    )
            });
    }

    protected submit(event: SubmitEvent): void {
        const decision = (event.submitter as HTMLButtonElement | null)?.value;
        if (this.submitting() || this.creatingSite() || this.expired() || !this.consent() || (decision !== 'approve' && decision !== 'deny') || (decision === 'approve' && !this.canApprove())) {
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
