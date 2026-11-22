import { ChangeDetectionStrategy, Component, computed, ElementRef, inject, input, output } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { injectActiveLang } from '@core/i18n/active-lang';
import { localeForLanguage } from '@core/i18n/duration-format';
import { KPI_DELTA_FORMAT, kpiDeltaClass } from './kpi-comparison-metric';

@Component({
    selector: 'button[appKpiComparisonTrigger]',
    imports: [TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    host: {
        type: 'button',
        'aria-haspopup': 'dialog',
        class: 'inline-flex min-h-11 min-w-11 max-w-full cursor-pointer items-center gap-1.5 rounded-md px-1 text-muted-color hover:bg-surface-100 focus-visible:outline focus-visible:outline-offset-[-2px] dark:hover:bg-surface-800',
        '[attr.aria-label]': 'accessibleLabel()',
        '(click)': 'compare.emit(element)'
    },
    template: `
        <span [class]="deltaClass() || 'text-xs font-medium'">{{ deltaLabel() || ('comparison.compare' | transloco) }}</span>
        <i class="pi pi-info-circle shrink-0 text-xs" aria-hidden="true"></i>
    `
})
export class KpiComparisonTrigger {
    metricLabel = input.required<string>();
    delta = input<number | null>(null);
    invertDelta = input(false);
    compare = output<HTMLButtonElement>();

    protected readonly element = inject<ElementRef<HTMLButtonElement>>(ElementRef).nativeElement;
    private readonly activeLanguage = injectActiveLang();
    private readonly transloco = inject(TranslocoService);

    protected readonly deltaClass = computed(() => kpiDeltaClass(this.delta(), this.invertDelta()));
    protected readonly deltaLabel = computed(() => {
        const delta = this.delta();
        return delta === null || !Number.isFinite(delta) ? '' : new Intl.NumberFormat(localeForLanguage(this.activeLanguage()), KPI_DELTA_FORMAT).format(delta / 100);
    });
    protected readonly accessibleLabel = computed(() => {
        this.activeLanguage();
        const label = this.transloco.translate('comparison.openMetric', { metric: this.metricLabel() });
        const delta = this.deltaLabel();
        return delta ? `${label} · ${delta}` : label;
    });
}
