import { ChangeDetectionStrategy, Component, computed, inject, input, signal } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { DialogShell } from '@components/dialog-shell/dialog-shell';
import type { ComparisonPeriod } from '@core/analytics/comparison-period';
import { injectActiveLang } from '@core/i18n/active-lang';
import { localeForLanguage } from '@core/i18n/duration-format';
import { ComparisonPeriods } from './comparison-periods';
import { KPI_DELTA_FORMAT, type KpiComparisonCard, kpiDelta } from './kpi-comparison-metric';

@Component({
    selector: 'app-kpi-comparison-dialog',
    imports: [ComparisonPeriods, DialogShell, TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <app-dialog-shell
            [title]="metric()?.label ?? ''"
            [visible]="open() && metric() !== null"
            (visibleChange)="$event || open.set(false)"
            (closed)="restoreFocus()"
            [secondaryLabel]="'common.actions.close' | transloco"
            [showPrimary]="false"
            width="32rem"
        >
            @if (metric(); as metric) {
                <div class="space-y-4 text-sm" data-testid="kpi-comparison">
                    <app-comparison-periods [current]="currentPeriod()" [previous]="metric.previousPeriod" [currentValue]="metric.current" [previousValue]="metric.previous" />
                    <dl class="grid grid-cols-2 gap-4 border-t border-surface-200 pt-4 dark:border-surface-700">
                        <div>
                            <dt class="text-muted-color">{{ 'comparison.absoluteChange' | transloco }}</dt>
                            <dd class="m-0 mt-0.5 font-semibold tabular-nums">{{ metric.absolute ?? ('comparison.unavailable' | transloco) }}</dd>
                        </div>
                        <div>
                            <dt class="text-muted-color">{{ 'comparison.relativeChange' | transloco }}</dt>
                            <dd class="m-0 mt-0.5 font-semibold tabular-nums">{{ metric.relative ?? ('comparison.unavailable' | transloco) }}</dd>
                        </div>
                    </dl>
                    @if (metric.zeroBaseline) {
                        <p class="text-muted-color">{{ 'comparison.zeroBaseline' | transloco }}</p>
                    }
                </div>
            }
        </app-dialog-shell>
    `
})
export class KpiComparisonDialog {
    cards = input.required<readonly KpiComparisonCard[]>();
    currentPeriod = input.required<ComparisonPeriod | null>();

    private readonly activeLanguage = injectActiveLang();
    private readonly transloco = inject(TranslocoService);
    private readonly selection = signal<{ id: string; trigger: HTMLButtonElement } | null>(null);
    protected readonly open = signal(false);

    protected readonly metric = computed(() => {
        const card = this.cards().find(({ id }) => id === this.selection()?.id);
        if (!this.currentPeriod() || !card || typeof card.value !== 'number' || !Number.isFinite(card.value)) return null;
        const locale = localeForLanguage(this.activeLanguage());
        const previous = card.previous;
        const delta = kpiDelta(card);
        return {
            label: card.label,
            current: this.formatValue(card, card.value, locale),
            previous: previous ? this.formatValue(card, previous.value, locale) : null,
            previousPeriod: previous?.period ?? null,
            absolute: previous ? this.formatValue(card, card.value - previous.value, locale, true) : null,
            relative: delta === null ? null : new Intl.NumberFormat(locale, KPI_DELTA_FORMAT).format(delta / 100),
            zeroBaseline: previous?.value === 0
        };
    });

    show(id: string, trigger: HTMLButtonElement): void {
        this.selection.set({ id, trigger });
        this.open.set(true);
    }

    protected restoreFocus(): void {
        const trigger = this.selection()?.trigger;
        if (trigger?.isConnected) trigger.focus({ preventScroll: true });
        this.open.set(false);
        this.selection.set(null);
    }

    private formatValue(card: KpiComparisonCard, value: number, locale: string, difference = false): string {
        const signDisplay = difference ? 'exceptZero' : 'auto';
        if (card.duration) {
            if (difference) {
                return new Intl.NumberFormat(locale, { style: 'unit', unit: 'second', unitDisplay: 'short', signDisplay, maximumFractionDigits: 1 }).format(value);
            }
            const wholeSeconds = Math.floor(value);
            const minutes = Math.floor(wholeSeconds / 60);
            const seconds = wholeSeconds % 60;
            return this.transloco.translate(minutes > 0 ? 'common.durationMinutesSeconds' : 'common.durationSeconds', { minutes, seconds });
        }
        if (card.suffix === '%' && !difference) {
            return new Intl.NumberFormat(locale, { ...card.format, style: 'percent' }).format(value / 100);
        }
        const formatted = new Intl.NumberFormat(locale, { ...card.format, signDisplay }).format(value);
        return card.suffix === '%' ? this.transloco.translate('comparison.percentagePoints', { value: formatted }) : `${card.prefix ?? ''}${formatted}${card.suffix ?? ''}`;
    }
}
