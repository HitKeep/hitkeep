import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';
import { formatCompactComparisonPeriod, formatComparisonPeriod, type ComparisonPeriod } from '@core/analytics/comparison-period';
import { injectActiveLang } from '@core/i18n/active-lang';
import { localeForLanguage } from '@core/i18n/duration-format';

@Component({
    selector: 'app-comparison-periods',
    changeDetection: ChangeDetectionStrategy.OnPush,
    imports: [TranslocoPipe],
    host: { class: 'block' },
    template: `
        <dl class="m-0 divide-y divide-surface-200 dark:divide-surface-700">
            @for (entry of periods(); track entry.label) {
                <div class="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 py-2">
                    <dt>
                        <span class="block font-medium">{{ entry.label | transloco }}</span>
                        @if (entry.compact) {
                            <span class="block text-muted-color">{{ entry.compact }}</span>
                        }
                    </dt>
                    <dd class="m-0 font-semibold tabular-nums" [class.text-muted-color]="entry.value === null">{{ entry.value ?? ('comparison.unavailable' | transloco) }}</dd>
                </div>
            }
        </dl>
        @if (current() || previous()) {
            <details class="mt-1 text-muted-color">
                <summary class="w-fit cursor-pointer py-1.5">{{ 'comparison.exactTimes' | transloco }}</summary>
                <dl class="m-0 grid gap-1 pb-1">
                    @for (entry of periods(); track entry.label) {
                        @if (entry.exact; as exact) {
                            <div class="flex flex-wrap items-baseline gap-x-2">
                                <dt class="font-medium">{{ entry.label | transloco }}</dt>
                                <dd class="m-0">{{ exact }}</dd>
                            </div>
                        }
                    }
                </dl>
                @if (current() && previous()) {
                    <p>{{ 'comparison.sameElapsedTime' | transloco }}</p>
                }
            </details>
        }
    `
})
export class ComparisonPeriods {
    readonly current = input<ComparisonPeriod | null>(null);
    readonly previous = input<ComparisonPeriod | null>(null);
    readonly currentValue = input<string | null>(null);
    readonly previousValue = input<string | null>(null);
    private readonly activeLanguage = injectActiveLang();

    protected readonly periods = computed(() => {
        const locale = localeForLanguage(this.activeLanguage());
        return [
            { label: 'comparison.currentPeriod', period: this.current(), value: this.currentValue() },
            { label: 'comparison.previousPeriod', period: this.previous(), value: this.previousValue() }
        ].map(({ label, period, value }) => ({
            label,
            value,
            compact: period ? formatCompactComparisonPeriod(period, locale) : null,
            exact: period ? formatComparisonPeriod(period, locale) : null
        }));
    });
}
