import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import type { ComparisonPeriod } from '@core/analytics/comparison-period';
import { KpiCard } from './kpi-card';
import { KpiComparisonDialog } from './kpi-comparison-dialog';
import { type KpiComparisonCard, kpiDelta } from './kpi-comparison-metric';

@Component({
    selector: 'app-kpi-comparison-cards',
    imports: [KpiCard, KpiComparisonDialog],
    changeDetection: ChangeDetectionStrategy.OnPush,
    host: { class: 'block' },
    template: `
        <div class="rounded-lg border border-[var(--p-content-border-color)] bg-[var(--p-content-background)]" [class]="gridClass()">
            @for (card of cards(); track card.id) {
                <app-kpi-card
                    [flat]="true"
                    [label]="card.label"
                    [value]="card.value"
                    [loading]="card.loading"
                    [updateKey]="card.updateKey ?? 0"
                    [valueClass]="card.valueClass ?? ''"
                    [format]="card.format"
                    [prefix]="card.prefix"
                    [suffix]="card.suffix"
                    [duration]="card.duration ?? false"
                    [invertDelta]="card.invertDelta ?? false"
                    [description]="card.description ?? ''"
                    [delta]="deltaFor(card)"
                    [comparable]="currentPeriod() !== null && card.previous !== undefined"
                    (compare)="comparison.show(card.id, $event)"
                />
            }
        </div>
        <app-kpi-comparison-dialog #comparison [cards]="cards()" [currentPeriod]="currentPeriod()" />
    `
})
export class KpiComparisonCards {
    cards = input.required<readonly KpiComparisonCard[]>();
    currentPeriod = input.required<ComparisonPeriod | null>();
    gridClass = input.required<string>();
    protected readonly deltaFor = kpiDelta;
}
