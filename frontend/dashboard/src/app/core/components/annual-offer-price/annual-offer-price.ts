import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { injectActiveLang } from '@core/i18n/active-lang';
import { AnnualOfferPrice } from '@services/cloud.service';

@Component({
    selector: 'app-annual-offer-price',
    imports: [TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <p class="m-0 text-xs leading-5 font-bold text-[var(--p-highlight-color)]">{{ 'cloud.annualOffer.saving' | transloco }}</p>
        <p class="m-0 flex flex-wrap items-baseline gap-x-2">
            <strong class="text-3xl leading-tight font-bold tracking-tight tabular-nums">{{ first() }}</strong>
            <span class="text-sm">{{ 'cloud.annualOffer.firstYear' | transloco }}</span>
        </p>
        <p class="m-0 mt-1 text-sm leading-5">{{ 'cloud.annualOffer.renewal' | transloco: { price: renewal() } }}</p>
        <p class="m-0 text-xs leading-5 text-muted-color">{{ 'cloud.annualOffer.deadline' | transloco: { date: deadline() } }}</p>
    `
})
export class AnnualOfferPriceDisplay {
    readonly price = input.required<AnnualOfferPrice>();
    readonly endsAt = input.required<string>();
    private readonly lang = injectActiveLang();
    private readonly locale = inject(TranslocoLocaleService);
    protected readonly first = computed(() => this.money(this.price().first_invoice_minor));
    protected readonly renewal = computed(() => this.money(this.price().renewal_minor));
    protected readonly deadline = computed(() => {
        this.lang();
        return this.locale.localizeDate(this.endsAt(), undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit', timeZoneName: 'short' });
    });

    private money(minor: number): string {
        this.lang();
        return this.locale.localizeNumber(minor / 100, 'currency', undefined, { currency: this.price().currency });
    }
}
