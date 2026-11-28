import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { injectActiveLang } from '@core/i18n/active-lang';
import { CopyControl } from '@components/copy-control/copy-control';
import { BillingInterval, PromotionPrice } from '@services/cloud.service';

@Component({
    selector: 'app-promotion-price',
    imports: [TranslocoPipe, CopyControl],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <p class="m-0 text-xs leading-5 font-bold text-[var(--p-highlight-color)]">{{ 'cloud.promotion.saving' | transloco: { percent: percent() } }}</p>
        <p class="m-0 flex flex-wrap items-baseline gap-x-2">
            <strong class="text-3xl leading-tight font-bold tracking-tight tabular-nums">{{ first() }}</strong>
            @if (monthlyEquivalent() && interval() === 'annual') {
                <span class="text-sm">{{ 'admin.team.overview.plans.perMonth' | transloco }}</span>
            }
            <span class="text-sm">{{ 'cloud.promotion.withCode' | transloco: { code: code() } }}</span>
            <app-copy-control [value]="code()" [text]="true" size="small" ariaLabelKey="cloud.promotion.copy" labelKey="cloud.promotion.copy" />
        </p>
        @if (monthlyEquivalent() && interval() === 'annual') {
            <p class="m-0 text-sm">{{ 'cloud.billing.annualTotal' | transloco: { price: invoiceTotal() } }}</p>
        }
        <p class="m-0 mt-1 text-sm leading-5">{{ (interval() === 'annual' ? 'cloud.promotion.annualRenewal' : 'cloud.promotion.monthlyRenewal') | transloco: { price: renewal() } }}</p>
        <p class="m-0 text-xs leading-5 text-muted-color">{{ (paid() ? 'cloud.promotion.applyDeadline' : 'cloud.promotion.checkoutDeadline') | transloco: { date: deadline() } }}</p>
    `
})
export class PromotionPriceDisplay {
    readonly monthlyEquivalent = input(false);
    readonly paid = input(false);
    readonly interval = input.required<BillingInterval>();
    readonly percentOff = input.required<number>();
    readonly code = input.required<string>();
    readonly price = input.required<PromotionPrice>();
    readonly endsAt = input.required<string>();
    private readonly lang = injectActiveLang();
    private readonly locale = inject(TranslocoLocaleService);
    protected readonly percent = computed(() => {
        this.lang();
        return this.locale.localizeNumber(this.percentOff() / 100, 'percent', undefined, { maximumFractionDigits: 2 });
    });
    protected readonly invoiceTotal = computed(() => this.money(this.price().with_code_minor));
    protected readonly first = computed(() => (this.monthlyEquivalent() && this.interval() === 'annual' ? this.money(this.price().with_code_minor / 12) : this.invoiceTotal()));
    protected readonly renewal = computed(() => this.money(this.price().regular_minor));
    protected readonly deadline = computed(() => {
        this.lang();
        return this.locale.localizeDate(this.endsAt(), undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit', timeZoneName: 'short' });
    });

    private money(minor: number): string {
        this.lang();
        return this.locale.localizeNumber(minor / 100, 'currency', undefined, { currency: this.price().currency });
    }
}
