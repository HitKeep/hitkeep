import { ChangeDetectionStrategy, Component, input, model } from '@angular/core';
import type { ECharts } from 'echarts/core';
import type { HitkeepChartSeries } from './hitkeep-chart-options';

@Component({
    selector: 'app-chart-legend',
    changeDetection: ChangeDetectionStrategy.OnPush,
    host: { '(document:keydown.escape)': 'dismissTooltip()' },
    template: `
        @if (series().length > 1) {
            <div class="mt-1 flex flex-wrap items-center justify-center gap-x-2">
                @for (item of series(); track item.id) {
                    <button type="button" class="inline-flex min-h-9 items-center gap-2 rounded px-2 text-xs text-color hover:bg-emphasis aria-[pressed=false]:line-through" [attr.aria-pressed]="!hidden().has(item.id)" (click)="toggle(item)">
                        <span aria-hidden="true" class="inline-block w-5 border-t-[3px]" [style.border-top-color]="item.color" [style.border-top-style]="item.muted ? 'dashed' : item.dashed ? 'dotted' : 'solid'"></span>
                        {{ item.label }}
                    </button>
                }
            </div>
        }
    `
})
export class ChartLegend {
    readonly series = input.required<HitkeepChartSeries[]>();
    readonly instance = model<ECharts | null>(null);
    readonly hidden = model<ReadonlySet<string>>(new Set());

    protected toggle(item: HitkeepChartSeries): void {
        const chart = this.instance();
        if (!chart || chart.isDisposed()) return;
        chart.dispatchAction({ type: 'legendToggleSelect', name: item.label });
    }

    syncSelection(selected: Record<string, boolean>): void {
        this.hidden.set(
            new Set(
                this.series()
                    .filter(({ label }) => selected[label] === false)
                    .map(({ id }) => id)
            )
        );
    }

    protected dismissTooltip(): void {
        const chart = this.instance();
        if (!chart || chart.isDisposed()) return;
        // ECharts ignores hideTip while its enterable HTML content is hovered.
        const tooltip = chart.getDom().querySelector<HTMLElement>('.hk-echarts-tooltip:hover');
        if (tooltip) tooltip.dispatchEvent(new MouseEvent('mouseleave'));
        chart.dispatchAction({ type: 'hideTip' });
    }
}
