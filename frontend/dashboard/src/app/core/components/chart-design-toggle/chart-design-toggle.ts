import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';
import type { HitkeepChartDesign } from '@core/charts/hitkeep-chart-options';

@Component({
    selector: 'app-chart-design-toggle',
    imports: [TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <div role="radiogroup" [attr.aria-label]="'common.chartDesign.label' | transloco" class="flex flex-wrap items-center gap-x-1">
            <span aria-hidden="true" class="mr-1 text-xs font-medium text-muted-color">{{ 'common.chartDesign.label' | transloco }}</span>
            @for (design of designs; track design) {
                <label class="inline-flex min-h-9 cursor-pointer items-center gap-1.5 rounded px-2 text-xs text-color hover:bg-emphasis has-focus-visible:outline-2 has-focus-visible:outline-offset-2">
                    <input type="radio" [name]="groupName" [value]="design" [checked]="value() === design" class="accent-[var(--p-primary-color)]" (change)="valueChange.emit(design)" />
                    {{ 'common.chartDesign.' + design | transloco }}
                </label>
            }
        </div>
    `
})
export class ChartDesignToggle {
    private static nextID = 0;
    protected readonly groupName = `chart-design-${ChartDesignToggle.nextID++}`;
    protected readonly designs: readonly HitkeepChartDesign[] = ['area', 'line', 'bar'];
    readonly value = input.required<HitkeepChartDesign>();
    readonly valueChange = output<HitkeepChartDesign>();
}
