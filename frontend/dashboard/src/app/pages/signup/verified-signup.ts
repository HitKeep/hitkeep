import { DOCUMENT } from '@angular/common';
import { ChangeDetectionStrategy, Component, DestroyRef, computed, inject, signal } from '@angular/core';
import { rxResource, takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { combineLatest, finalize, map } from 'rxjs';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { MessageModule } from '@openng/optimus-ui/message';

import { Brand } from '@components/brand/brand';
import { AuthCard } from '@core/components/auth-card/auth-card';
import { injectActiveLang } from '@core/i18n/active-lang';
import { TeamService } from '@services/team.service';
import { PromotionPriceDisplay } from '@components/promotion-price/promotion-price';
import { BillingInterval, CloudService } from '@services/cloud.service';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { OAuthService } from '@services/oauth.service';
import { CloudSignupTrackingService } from '@services/cloud-signup-tracking.service';

type PaidPlan = 'pro' | 'business';

@Component({
    selector: 'app-verified-signup',
    imports: [AuthCard, Brand, RouterLink, PromotionPriceDisplay, ButtonModule, MessageModule, TranslocoPipe],
    template: `
        <main class="hk-auth-page">
            <section class="hk-auth-shell" aria-labelledby="verified-signup-title">
                <app-auth-card>
                    <header class="hk-auth-header">
                        <app-brand size="large" />
                        <h1 id="verified-signup-title" class="hk-auth-title">
                            {{ 'signup.verified.title' | transloco }}
                        </h1>
                        <p class="hk-auth-subtitle">
                            {{
                                'signup.verified.subtitle'
                                    | transloco
                                        : {
                                              plan: 'signup.plans.' + plan() + '.name' | transloco,
                                              billing: 'signup.billing.' + billing() | transloco
                                          }
                            }}
                        </p>
                    </header>

                    <div class="hk-auth-form" aria-live="polite">
                        @if (cloudReady()) {
                            @if (review.isLoading()) {
                                <p-message severity="info">{{ 'signup.checkingCloud' | transloco }}</p-message>
                            } @else if (alreadyPaid()) {
                                <p-message severity="success">{{ 'signup.recovery.paidTitle' | transloco }}</p-message>
                            } @else if (priceLabel() && !checkoutFailed()) {
                                @let offer = review.value()?.state?.promotion;
                                <strong>{{ review.value()?.team?.name }}</strong>
                                @if (offer && offer.prices[plan()]?.[billing()]; as price) {
                                    <app-promotion-price [price]="price" [code]="offer!.code" [endsAt]="offer!.ends_at" [percentOff]="offer!.percent_off" [interval]="billing()" />
                                }
                                <p-button
                                    [label]="'signup.recovery.continuePaid' | transloco: { plan: 'signup.plans.' + plan() + '.name' | transloco, price: priceLabel() }"
                                    [loading]="checkoutPending()"
                                    [disabled]="checkoutPending() || oauth.resuming()"
                                    [fluid]="true"
                                    (onClick)="startCheckout()"
                                />
                                <p class="m-0 text-xs text-muted-color">{{ 'cloud.billing.priceNotice' | transloco }}</p>
                            } @else {
                                <p-message severity="warn">{{ 'signup.verified.checkoutFailed' | transloco }}</p-message>
                                <a pButton routerLink="/admin/team/overview" severity="secondary"
                                    ><span pButtonLabel>{{ 'nav.teamOverview' | transloco }}</span></a
                                >
                            }
                            @if (alreadyPaid()) {
                                <a pButton routerLink="/dashboard"
                                    ><span pButtonLabel>{{ 'signup.recovery.dashboard' | transloco }}</span></a
                                >
                            } @else {
                                <p-button
                                    [label]="'signup.verified.continueFree' | transloco"
                                    [disabled]="checkoutPending() || oauth.resuming()"
                                    [loading]="oauth.resuming()"
                                    severity="secondary"
                                    [text]="true"
                                    [fluid]="true"
                                    (onClick)="continueFree()"
                                />
                            }
                        } @else {
                            <p-message severity="info">{{ 'signup.checkingCloud' | transloco }}</p-message>
                        }
                        <p class="m-0 text-center text-xs text-[var(--p-text-muted-color)]">
                            {{ 'signup.verified.freeNote' | transloco }}
                        </p>
                    </div>
                </app-auth-card>
            </section>
        </main>
    `,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class VerifiedSignup {
    private readonly document = inject(DOCUMENT);
    private readonly route = inject(ActivatedRoute);
    private readonly cloud = inject(CloudService);
    private readonly teams = inject(TeamService);
    private readonly locale = inject(TranslocoLocaleService);
    protected readonly oauth = inject(OAuthService);
    private readonly destroyRef = inject(DestroyRef);
    protected readonly cloudReady = inject(DashboardBootstrapService).cloudHosted;
    private readonly tracking = inject(CloudSignupTrackingService);
    private readonly activeLanguage = injectActiveLang();

    protected readonly plan = signal<PaidPlan>(this.normalizePlan(this.route.snapshot.queryParamMap.get('plan')));
    protected readonly billing = signal<BillingInterval>(this.normalizeBilling(this.route.snapshot.queryParamMap.get('billing')));

    protected readonly checkoutPending = signal(false);
    protected readonly checkoutFailed = signal(false);
    protected readonly review = rxResource({
        params: () => (this.cloudReady() ? (this.teams.activeTeam() ?? undefined) : undefined),
        stream: ({ params: team }) =>
            combineLatest({ plans: this.cloud.getPlans(), state: this.cloud.watchBillingState(team.id) }).pipe(
                map(({ plans, state }) => ({
                    state,
                    team: state.team_id === team.id ? team : undefined,
                    price: plans.find((tier) => tier.code === this.plan())?.prices?.[this.billing()]
                }))
            )
    });

    protected readonly alreadyPaid = computed(() => this.review.hasValue() && !!this.review.value().team && this.review.value().state.plan_code !== 'free');
    protected readonly priceLabel = computed(() => {
        this.activeLanguage();
        const data = this.review.hasValue() ? this.review.value() : undefined;
        if (!data?.team || !data.price || data.state.plan_code !== 'free') return null;
        return this.locale.localizeNumber(data.price.amount_minor / 100, 'currency', undefined, { currency: data.price.currency });
    });

    constructor() {
        if (!this.cloudReady()) {
            this.oauth.continueAfterAuthentication('/dashboard');
            return;
        }
        this.tracking.install();
        this.track('signup_verified');
    }

    protected startCheckout(): void {
        if (!this.priceLabel() || this.checkoutPending() || this.oauth.resuming()) return;
        const state = this.review.value()!.state;
        const offer = state.promotion;
        this.checkoutPending.set(true);
        this.cloud
            .createBillingCheckoutSession({
                team_id: state.team_id,
                plan_code: this.plan(),
                billing: this.billing(),
                locale: this.activeLanguage(),
                ...(offer?.prices[this.plan()]?.[this.billing()] ? { expected_promotion: offer.code } : {})
            })
            .pipe(
                finalize(() => this.checkoutPending.set(false)),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: ({ url }) => this.document.defaultView?.location.assign(url),
                error: () => this.checkoutFailed.set(true)
            });
    }

    protected continueFree(): void {
        this.track('continue_on_free');
        this.oauth.continueAfterAuthentication('/dashboard');
    }

    private track(name: string): void {
        this.tracking.trackEvent(name, {
            plan: this.plan(),
            interval: this.billing(),
            jurisdiction: this.inferJurisdiction(),
            source_path: '/signup/verified'
        });
    }

    private normalizePlan(value: string | null): PaidPlan {
        return value?.trim().toLowerCase() === 'business' ? 'business' : 'pro';
    }

    private normalizeBilling(value: string | null): BillingInterval {
        return value?.trim().toLowerCase() === 'annual' ? 'annual' : 'monthly';
    }

    private inferJurisdiction(): 'EU' | 'US' {
        const hostname = this.document.defaultView?.location.hostname.toLowerCase() ?? '';
        return hostname === 'cloud.hitkeep.com' || hostname.endsWith('.hitkeep.com') ? 'US' : 'EU';
    }
}
