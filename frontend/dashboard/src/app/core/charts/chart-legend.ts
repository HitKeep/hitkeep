import { TranslocoPipe } from '@jsverse/transloco';
import { ChangeDetectionStrategy, Component, input, model } from '@angular/core';
import type { ECharts } from 'echarts/core';
import type { HitkeepChartSeries } from './hitkeep-chart-options';

@Component({
    selector: 'app-chart-legend',
    changeDetection: ChangeDetectionStrategy.OnPush,
    imports: [TranslocoPipe],
    host: { '(document:keydown.escape)': 'dismissTooltip()' },
    template: `
        @if (series().length > 1) {
            <div role="group" [attr.aria-label]="'common.chartDesign.series' | transloco" class="mt-1 flex flex-wrap items-center gap-1">
                @for (item of series(); track item.id) {
                    <button
                        type="button"
                        class="inline-flex min-h-9 max-w-full items-center gap-2 rounded border border-transparent px-2 text-left text-xs text-color hover:bg-emphasis aria-[pressed=true]:border-[var(--p-content-border-color)] aria-[pressed=false]:line-through focus-visible:outline-2 focus-visible:outline-offset-2"
                        [attr.aria-pressed]="!hidden().has(item.id)"
                        (click)="toggle(item)"
                    >
                        <span aria-hidden="true" class="inline-block w-4 shrink-0 border-t-[3px]" [style.border-top-color]="item.color" [style.border-top-style]="item.muted ? 'dashed' : item.dashed ? 'dotted' : 'solid'"></span>
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
