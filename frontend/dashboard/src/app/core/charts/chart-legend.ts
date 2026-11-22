import { NgTemplateOutlet } from '@angular/common';
import { TranslocoPipe } from '@jsverse/transloco';
import { ChangeDetectionStrategy, Component, computed, input, model } from '@angular/core';
import type { ECharts } from 'echarts/core';
import type { HitkeepChartSeries } from './hitkeep-chart-options';

@Component({
    selector: 'app-chart-legend',
    changeDetection: ChangeDetectionStrategy.OnPush,
    imports: [NgTemplateOutlet, TranslocoPipe],
    host: { '(document:keydown.escape)': 'dismissTooltip()' },
    template: `
        <ng-template #itemButton let-item>
            <button type="button" class="inline-flex min-h-9 items-center gap-2 rounded px-2 text-xs text-color hover:bg-emphasis aria-[pressed=false]:line-through" [attr.aria-pressed]="!hidden().has(item.id)" (click)="toggle(item)">
                <span aria-hidden="true" class="inline-block w-5 border-t-[3px]" [style.border-top-color]="item.color" [style.border-top-style]="item.muted ? 'dashed' : item.dashed ? 'dotted' : 'solid'"></span>
                {{ item.label }}
            </button>
        </ng-template>
        <div class="mt-1 flex flex-wrap items-start justify-between gap-x-3">
            @if (series().length > 1) {
                <div class="flex flex-wrap items-center gap-x-1">
                    @for (item of visibleKeys(); track item.id) {
                        <ng-container [ngTemplateOutlet]="itemButton" [ngTemplateOutletContext]="{ $implicit: item }" />
                    }
                </div>
            }
            @if (toolsAvailable() || overlays().length) {
                <details class="ml-auto max-w-full">
                    <summary class="ml-auto w-fit cursor-pointer rounded px-2 py-2 text-xs font-medium text-muted-color">{{ 'common.chartDesign.options' | transloco }}</summary>
                    @if (overlays().length) {
                        <fieldset class="mt-1 flex flex-wrap gap-x-1 rounded border border-[var(--p-content-border-color)] px-2 pb-1">
                            <legend class="px-1 text-xs text-muted-color">{{ 'common.chartDesign.overlays' | transloco }}</legend>
                            @for (item of overlays(); track item.id) {
                                <ng-container [ngTemplateOutlet]="itemButton" [ngTemplateOutletContext]="{ $implicit: item }" />
                            }
                        </fieldset>
                    }
                    <ng-content />
                </details>
            }
        </div>
    `
})
export class ChartLegend {
    readonly series = input.required<HitkeepChartSeries[]>();
    readonly instance = model<ECharts | null>(null);
    readonly hidden = model<ReadonlySet<string>>(new Set());

    readonly toolsAvailable = input(false);
    protected readonly overlays = computed(() => this.series().filter((item) => item.muted || item.dashed));
    protected readonly visibleKeys = computed(() => this.series().filter((item) => (!item.muted && !item.dashed) || !this.hidden().has(item.id)));

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
