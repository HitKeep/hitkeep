import { ChangeDetectionStrategy, Component, DestroyRef, computed, inject, signal } from '@angular/core';
import { rxResource, takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { NgTemplateOutlet } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { EMPTY, catchError, exhaustMap, ignoreElements, merge, finalize, of, switchMap, takeUntil, takeWhile, tap, timer } from 'rxjs';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { ButtonModule } from '@openng/optimus-ui/button';
import { InputText } from '@openng/optimus-ui/inputtext';
import { CardModule } from '@openng/optimus-ui/card';
import { ProgressBarModule } from '@openng/optimus-ui/progressbar';
import { SelectButtonModule } from '@openng/optimus-ui/selectbutton';
import { TagModule } from '@openng/optimus-ui/tag';
import { PromotionPriceDisplay } from '@components/promotion-price/promotion-price';
import { SettingsCard } from '@features/settings/components/settings-card';
import { TeamService } from '@services/team.service';
import { injectActiveLang } from '@core/i18n/active-lang';
import { AnalyticsService } from '@services/analytics.service';
import { OAuthService } from '@services/oauth.service';
import { billingStateUntilExpiry, BillingInterval, CloudService } from '@services/cloud.service';
import { cloudPurchaseIntent, type CloudPurchaseIntent } from '@core/utils/cloud-purchase-intent';

import { CloudPlanTier, TeamPlan, TeamRole } from '@models/analytics.types';
import { AdminItemList, type AdminItemListEntry } from '../components/admin-item-list';

const PLAN_RANK: Record<string, number> = { free: 0, pro: 1, business: 2 };

@Component({
    selector: 'app-team-overview',
    imports: [NgTemplateOutlet, InputText, PromotionPriceDisplay, AdminItemList, ButtonModule, CardModule, FormsModule, ProgressBarModule, RouterLink, SelectButtonModule, SettingsCard, TagModule, TranslocoPipe],
    templateUrl: './team-overview.html',
    styleUrl: './team-overview.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class TeamOverviewPage {
    private readonly destroyRef = inject(DestroyRef);
    private readonly route = inject(ActivatedRoute);
    private readonly router = inject(Router);
    private readonly transloco = inject(TranslocoService);
    private readonly localeService = inject(TranslocoLocaleService);
    private readonly activeLanguage = injectActiveLang();
    private readonly analyticsService = inject(AnalyticsService);
    private readonly cloudService = inject(CloudService);
    protected readonly oauth = inject(OAuthService);
    protected readonly teamService = inject(TeamService);

    protected readonly systemStatusResource = rxResource({
        stream: () => this.analyticsService.getSystemStatus()
    });
    protected readonly systemStatus = computed(() => (this.systemStatusResource.hasValue() ? this.systemStatusResource.value() : null));
    protected readonly team = this.teamService.activeTeam;
    private readonly recoveryQuery = this.route.snapshot.queryParamMap;
    protected readonly checkoutReturned = this.recoveryQuery.get('checkout') === 'success';
    protected readonly recoveryMode = this.recoveryQuery.get('checkout') === 'canceled' ? 'canceled' : this.recoveryQuery.get('purchase') === 'review' ? 'review' : null;
    private readonly requestedIntent = cloudPurchaseIntent(this.recoveryQuery.get('plan'), this.recoveryQuery.get('billing'));
    protected readonly returnTeamId = this.recoveryQuery.get('team') ?? (this.checkoutReturned ? this.teamService.activeTeamId() : null);
    protected readonly confirmedTeamId = signal('');
    protected readonly recoveryError = signal(false);
    protected readonly confirmationFinished = signal(false);
    protected readonly billingStateResource = rxResource({
        params: () => ((this.recoveryMode || this.checkoutReturned || this.team()?.role === 'owner' || this.team()?.role === 'admin') && this.systemStatus()?.cloud?.hosted ? this.teamService.activeTeamId() || undefined : undefined),
        stream: ({ params: team }) => {
            if (!this.checkoutReturned || (this.returnTeamId && this.returnTeamId !== team)) return this.cloudService.watchBillingState(team);
            this.confirmationFinished.set(false);
            return timer(0, 2500).pipe(
                exhaustMap(() => this.cloudService.getBillingState({ sync: true })),
                takeWhile((state) => state.team_id === team && state.plan_code === 'free', true),
                takeUntil(timer(15_000)),
                tap({ complete: () => this.confirmationFinished.set(true) }),
                switchMap((state) => {
                    const confirmed = billingStateUntilExpiry(of(state));
                    if (state.team_id !== team || state.plan_code === 'free' || state.plan_code === this.team()?.plan?.code) return confirmed;
                    return merge(
                        confirmed,
                        this.teamService.loadTeams().pipe(
                            ignoreElements(),
                            catchError(() => EMPTY)
                        )
                    );
                })
            );
        }
    });
    protected readonly billingState = computed(() => (this.billingStateResource.hasValue() ? this.billingStateResource.value() : null));
    protected readonly promotion = computed(() => {
        const state = this.billingState();
        return state && state.team_id === this.team()?.id ? state.promotion : undefined;
    });
    protected offerPrice(plan: string, interval: BillingInterval) {
        return this.promotion()?.prices[plan]?.[interval];
    }

    private offerExpectation(plan: string, interval: BillingInterval) {
        const offer = this.promotion();
        return offer?.prices[plan]?.[interval] ? { team_id: this.teamService.activeTeamId(), expected_promotion: offer.code } : {};
    }

    protected readonly paidPrice = computed(() => {
        const state = this.billingState();
        if (!state || state.plan_code === 'free' || (this.returnTeamId && this.returnTeamId !== state.team_id)) return null;
        const prices = this.promotion()?.prices[state.plan_code];
        const interval: BillingInterval = prices?.annual ? 'annual' : 'monthly';
        const price = prices?.[interval];
        return price ? ({ price, interval } as const) : null;
    });
    protected readonly redeemPending = signal(false);
    protected readonly redemption = signal<{ team: string; status: 'applied' | 'failed' } | null>(null);
    protected redeemCode(code: string): void {
        const team = this.teamService.activeTeamId();
        if (!this.paidPrice() || !code.trim() || this.redeemPending() || (this.returnTeamId && this.returnTeamId !== team)) return;
        this.redeemPending.set(true);
        this.cloudService
            .redeemPromotion(team, code.trim())
            .pipe(
                finalize(() => this.redeemPending.set(false)),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: () => {
                    this.redemption.set({ team, status: 'applied' });
                    if (this.checkoutReturned) this.refreshBilling();
                },
                error: () => {
                    this.redemption.set({ team, status: 'failed' });
                    this.refreshBilling();
                }
            });
    }
    protected readonly checkoutAuthorizationReady = computed(() => {
        const state = this.billingState();
        // Reuse the native paid-plan rule; the checkout query is not payment proof.
        return this.checkoutReturned && (!this.returnTeamId || this.returnTeamId === state?.team_id) && !!state && state.team_id === this.teamService.activeTeamId() && state.plan_code !== 'free';
    });
    protected readonly authorizationContinuation = rxResource({
        params: () => (this.checkoutAuthorizationReady() ? this.teamService.activeTeamId() : undefined),
        stream: () => this.oauth.continuation()
    });
    protected readonly recoveryKind = computed(() => {
        if (!this.recoveryMode || !this.systemStatus()?.cloud?.hosted || this.requestedIntent.plan === 'free') return null;
        if ((this.recoveryMode === 'canceled' || this.returnTeamId) && this.returnTeamId !== this.teamService.activeTeamId()) return 'switchTeam';
        if (this.billingStateResource.isLoading()) return 'loading';
        const state = this.billingState();
        if (!state || state.team_id !== this.teamService.activeTeamId()) return 'unavailable';
        if (state.plan_code !== 'free') return 'paid';
        if (this.recoveryMode === 'canceled') return state.subscription_status === 'pending_checkout' && state.pending_plan_code && state.billing ? 'canceled' : 'unavailable';
        return 'review';
    });
    protected readonly recoveryIntent = computed<CloudPurchaseIntent>(() => {
        const state = this.billingState();
        return this.recoveryMode === 'canceled' && state?.subscription_status === 'pending_checkout' && state.pending_plan_code ? cloudPurchaseIntent(state.pending_plan_code, state.billing) : this.requestedIntent;
    });
    protected readonly recoveryPrice = computed(() => {
        this.activeLanguage();
        const intent = this.recoveryIntent();

        const price = this.planTiers().find((tier) => tier.code === intent.plan)?.prices?.[intent.billing];
        return price ? this.localeService.localizeNumber(price.amount_minor / 100, 'currency', undefined, { currency: price.currency }) : this.transloco.translate('signup.priceUnavailable');
    });
    protected readonly teamConfirmed = computed(() => !this.teamService.hasMultipleTeams() || this.confirmedTeamId() === this.teamService.activeTeamId());
    protected readonly planTiersResource = rxResource({
        params: () => (this.systemStatus()?.cloud?.hosted ? true : undefined),
        stream: () => this.cloudService.getPlans()
    });
    protected readonly planTiers = computed<CloudPlanTier[]>(() => (this.planTiersResource.hasValue() ? this.planTiersResource.value() : []));
    protected readonly portalPending = signal(false);
    protected readonly portalError = signal(false);
    protected readonly checkoutPlanPending = signal<'pro' | 'business' | null>(null);
    protected readonly freePending = signal(false);
    protected readonly checkoutPending = computed(() => !!this.checkoutPlanPending() || this.freePending());
    protected readonly billingInterval = signal<BillingInterval>(this.recoveryMode ? this.requestedIntent.billing : 'annual');
    protected readonly billingOptions = computed(() => {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('admin.team.overview.plans.monthly'), value: 'monthly' as const },
            { label: this.transloco.translate('admin.team.overview.plans.annual'), value: 'annual' as const }
        ];
    });
    protected readonly teamDetailItems = computed<readonly AdminItemListEntry[]>(() => {
        this.activeLanguage();
        const team = this.team();
        if (!team) return [];
        return [
            {
                id: 'team-id',
                label: this.transloco.translate('admin.team.overview.teamIdLabel'),
                description: team.id,
                descriptionMonospace: true,
                copyValue: team.id
            },
            {
                id: 'role',
                label: this.transloco.translate('common.columns.role'),
                meta: this.roleLabel(team.role)
            },
            {
                id: 'members',
                label: this.transloco.translate('admin.team.overview.memberCount'),
                meta: this.localeService.localizeNumber(team.usage?.current_members ?? 0, 'decimal')
            }
        ];
    });
    protected readonly usageCards = computed(() => {
        this.activeLanguage();
        const team = this.team();
        const cloud = this.systemStatus()?.cloud;
        if (!cloud?.hosted || !team?.usage || !team.entitlements) {
            return [];
        }

        const cards = [this.buildUsageCard('sites', team.usage.current_sites, team.entitlements.max_sites_per_team), this.buildUsageCard('members', team.usage.current_members, team.entitlements.max_team_members)];
        if (team.entitlements.max_monthly_hits && team.entitlements.max_monthly_hits > 0) cards.push(this.buildUsageCard('monthlyHits', team.usage.current_monthly_hits ?? 0, team.entitlements.max_monthly_hits));
        return cards;
    });
    protected readonly cloudPlan = computed(() => {
        const team = this.team();
        const cloud = this.systemStatus()?.cloud;
        if (!cloud?.hosted || !team?.plan || !team.entitlements) {
            return null;
        }

        return {
            plan: team.plan,
            cloud,
            retentionDays: team.entitlements.max_retention_days
        };
    });
    protected readonly cloudPlanItems = computed<readonly AdminItemListEntry[]>(() => {
        this.activeLanguage();
        const plan = this.cloudPlan();
        if (!plan) return [];
        const items: AdminItemListEntry[] = [
            {
                id: 'plan',
                label: this.transloco.translate('admin.team.overview.plans.currentPlan'),
                meta: this.planName(plan.plan)
            }
        ];
        if (plan.cloud.jurisdiction) {
            items.push({ id: 'jurisdiction', label: this.transloco.translate('admin.team.overview.cloud.jurisdiction'), meta: plan.cloud.jurisdiction });
        }
        if (plan.cloud.region) {
            items.push({ id: 'region', label: this.transloco.translate('admin.team.overview.cloud.region'), meta: plan.cloud.region });
        }
        items.push({ id: 'retention', label: this.transloco.translate('admin.team.overview.cloud.retention'), meta: this.retentionLabel(plan.retentionDays) });
        return items;
    });
    protected readonly showUsageSection = computed(() => this.usageCards().length > 0);
    protected readonly canManageBilling = computed(() => {
        const plan = this.cloudPlan()?.plan;
        return Boolean(plan && plan.code !== 'free' && !this.isOperatorPlan(plan) && !this.portalPending());
    });
    protected readonly currentTier = computed(() => {
        const code = this.cloudPlan()?.plan.code;
        return this.planTiers().find((t) => t.code === code) ?? null;
    });
    protected readonly upgradeTiers = computed(() => {
        const currentRank = PLAN_RANK[this.cloudPlan()?.plan.code ?? ''];
        if (currentRank === undefined) {
            return [];
        }
        return this.planTiers().filter((tier) => (PLAN_RANK[tier.code] ?? -1) > currentRank);
    });
    protected readonly showPlanComparison = computed(() => this.upgradeTiers().length > 0);
    protected readonly comparisonTiers = computed(() => this.planTiers().filter((tier) => tier === this.currentTier() || this.upgradeTiers().includes(tier)));
    protected readonly activatedTier = computed(() => this.planTiers().find((tier) => tier.code === this.billingState()?.plan_code));
    protected readonly features = computed(() => {
        this.activeLanguage();
        return Object.fromEntries(
            this.planTiers().map((tier) => {
                const e = tier.entitlements;
                const label = (key: string, count?: number) => this.transloco.translate('admin.team.overview.plans.features.' + key, { count: count?.toLocaleString(this.activeLanguage()) });
                return [
                    tier.code,
                    [
                        ...(e.max_monthly_hits ? [{ label: label('monthlyHits', e.max_monthly_hits), included: true }] : []),
                        { label: label('sites', e.max_sites_per_team), included: true },
                        { label: label('members', e.max_team_members), included: true },
                        { label: this.retentionYears(e.max_retention_days), included: true },
                        ...(e.max_ask_ai_answers_per_day ? [{ label: label('askAi', e.max_ask_ai_answers_per_day), included: true }] : []),
                        { label: label('externalReportRecipients'), included: e.allow_external_report_recipients },
                        { label: label('customDomains'), included: tier.code !== 'free' },
                        { label: label('sso'), included: e.allow_sso },
                        { label: label('branding'), included: e.allow_custom_branding }
                    ]
                ] as const;
            })
        );
    });
    protected readonly planPriceLabels = computed<Record<string, string>>(() => {
        this.activeLanguage();
        const labels: Record<string, string> = {};
        for (const tier of this.planTiers()) {
            const price = tier.prices?.[this.billingInterval()];
            labels[tier.code] =
                tier.code === 'free'
                    ? this.transloco.translate('signup.freePrice')
                    : price
                      ? this.localeService.localizeNumber(price.amount_minor / (this.billingInterval() === 'annual' ? 1200 : 100), 'currency', undefined, { currency: price.currency })
                      : this.transloco.translate('signup.priceUnavailable');
        }
        return labels;
    });
    protected annualTotal(tier: CloudPlanTier): string {
        this.activeLanguage();
        const price = tier.prices?.annual;
        return price ? this.localeService.localizeNumber(price.amount_minor / 100, 'currency', undefined, { currency: price.currency }) : '';
    }

    protected roleSeverity(role: TeamRole): 'danger' | 'info' | 'secondary' {
        switch (role) {
            case 'owner':
                return 'danger';
            case 'admin':
                return 'info';
            case 'member':
            default:
                return 'secondary';
        }
    }

    protected roleLabel(role: TeamRole): string {
        return this.transloco.translate(`teams.roles.${role}`);
    }

    protected usageDescription(current: number): string {
        return this.transloco.translate('admin.team.overview.usage.currentUsage', {
            count: current.toLocaleString(this.activeLanguage())
        });
    }

    protected usageLimitLabel(limit: number): string {
        if (limit <= 0) {
            return this.transloco.translate('admin.team.overview.usage.unlimited');
        }
        return this.transloco.translate('admin.team.overview.usage.limitValue', {
            count: limit.toLocaleString(this.activeLanguage())
        });
    }

    protected usageStateClass(percentage: number, limit: number): string {
        if (limit <= 0) {
            return 'team-overview__usage-progress team-overview__usage-progress--unlimited';
        }
        if (percentage >= 95) {
            return 'team-overview__usage-progress team-overview__usage-progress--critical';
        }
        if (percentage >= 80) {
            return 'team-overview__usage-progress team-overview__usage-progress--warning';
        }
        return 'team-overview__usage-progress team-overview__usage-progress--healthy';
    }

    protected pendingInviteLabel(count: number): string {
        if (count === 1) {
            return this.transloco.translate('admin.team.overview.usage.pendingInviteOne');
        }
        return this.transloco.translate('admin.team.overview.usage.pendingInviteMany', { count });
    }

    protected retentionLabel(days: number): string {
        if (days <= 0) {
            return this.transloco.translate('admin.team.overview.cloud.unlimitedRetention');
        }
        return this.transloco.translate('admin.team.overview.cloud.retentionDays', {
            count: days
        });
    }

    protected planName(plan: TeamPlan): string {
        this.activeLanguage();
        if (this.isOperatorPlan(plan)) {
            return this.transloco.translate('admin.team.overview.cloud.operatorPlan');
        }
        return plan.name;
    }

    protected cloudPlanDescription(plan: TeamPlan): string {
        this.activeLanguage();
        if (this.isOperatorPlan(plan)) {
            return this.transloco.translate('admin.team.overview.cloud.operatorDescription');
        }
        return this.transloco.translate('admin.team.overview.cloud.description');
    }

    protected openBillingPortal(): void {
        if (this.portalPending()) {
            return;
        }

        this.portalError.set(false);
        this.portalPending.set(true);
        this.cloudService
            .createBillingPortalSession({ locale: this.activeLanguage() })
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: ({ url }) => {
                    this.portalPending.set(false);
                    this.redirectTo(url);
                },
                error: () => {
                    this.portalError.set(true);
                    this.portalPending.set(false);
                }
            });
    }

    protected confirmRecoveryTeam(): void {
        this.confirmedTeamId.set(this.teamService.activeTeamId());
    }

    protected startRecoveryCheckout(): void {
        const kind = this.recoveryKind();
        if ((kind !== 'canceled' && kind !== 'review') || !this.teamConfirmed() || this.checkoutPending()) return;
        const intent = this.recoveryIntent();
        if (intent.plan === 'free') return;
        const expected = this.offerExpectation(intent.plan, intent.billing);
        this.checkoutPlanPending.set(intent.plan);
        this.recoveryError.set(false);
        this.cloudService
            .getBillingState()
            .pipe(
                switchMap((state) => {
                    if (
                        state.team_id !== this.teamService.activeTeamId() ||
                        state.plan_code !== 'free' ||
                        (kind === 'canceled' && (state.subscription_status !== 'pending_checkout' || state.pending_plan_code !== intent.plan || state.billing !== intent.billing))
                    ) {
                        this.refreshBilling();
                        return EMPTY;
                    }
                    return this.cloudService.createBillingCheckoutSession({ ...expected, plan_code: intent.plan as 'pro' | 'business', billing: intent.billing, locale: this.activeLanguage() });
                }),
                finalize(() => this.checkoutPlanPending.set(null)),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({ next: ({ url }) => this.redirectTo(url), error: () => this.recoveryError.set(true) });
    }

    protected continueOnFree(): void {
        if (!this.teamConfirmed() || this.checkoutPending() || this.oauth.resuming()) return;
        this.freePending.set(true);
        this.recoveryError.set(false);
        this.cloudService
            .getBillingState()
            .pipe(
                finalize(() => this.freePending.set(false)),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: (state) => {
                    if (state.team_id !== this.teamService.activeTeamId() || state.plan_code !== 'free') {
                        this.refreshBilling();
                        return;
                    }
                    this.oauth.continueAfterAuthentication('/dashboard');
                },
                error: () => this.recoveryError.set(true)
            });
    }

    protected continueToAuthorization(): void {
        if (!this.checkoutAuthorizationReady() || !this.authorizationContinuation.hasValue() || !this.authorizationContinuation.value().pending) return;
        this.oauth.continueAfterAuthentication('/admin/team/overview');
    }

    protected refreshBilling(): void {
        if (this.checkoutReturned || this.billingStateResource.error()) this.billingStateResource.reload();
        else this.cloudService.refreshBillingState(this.teamService.activeTeamId());
    }

    protected startCheckoutForPlan(planCode: 'pro' | 'business'): void {
        if (this.checkoutPending()) {
            return;
        }

        this.checkoutPlanPending.set(planCode);
        this.cloudService
            .createBillingCheckoutSession({
                ...this.offerExpectation(planCode, this.billingInterval()),
                plan_code: planCode,
                billing: this.billingInterval(),
                locale: this.activeLanguage()
            })
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: ({ url }) => {
                    this.checkoutPlanPending.set(null);
                    this.redirectTo(url);
                },
                error: () => {
                    this.checkoutPlanPending.set(null);
                    this.recoveryError.set(true);
                    this.refreshBilling();
                }
            });
    }

    protected startUpgradeCheckout(): void {
        this.startCheckoutForPlan('pro');
    }

    private isOperatorPlan(plan: TeamPlan): boolean {
        return plan.code === 'operator';
    }

    private buildUsageCard(key: string, current: number, limit: number) {
        const percentage = limit > 0 ? Math.min(100, Math.floor((current / limit) * 100)) : 0;

        return {
            key,
            current,
            limit,
            displayValue: this.localeService.localizeNumber(current, 'decimal'),
            percentage,
            className: this.usageStateClass(percentage, limit),
            hasFiniteLimit: limit > 0,
            limitLabel: this.usageLimitLabel(limit),
            description: this.usageDescription(current)
        };
    }

    protected retentionYears(days: number): string {
        if (days <= 0) {
            return this.transloco.translate('admin.team.overview.usage.unlimited');
        }
        if (days < 365) {
            return this.transloco.translate('admin.team.overview.plans.features.retentionDays', { count: days });
        }
        return this.transloco.translate('admin.team.overview.plans.features.retention', { count: Math.round(days / 365) });
    }

    protected featureValue(value: number | boolean, suffix?: string): string {
        if (typeof value === 'boolean') {
            return '';
        }
        if (value <= 0) {
            return this.transloco.translate('admin.team.overview.usage.unlimited');
        }
        return suffix ? `${value} ${suffix}` : `${value}`;
    }

    protected redirectTo(url: string): void {
        window.location.assign(url);
    }
}
