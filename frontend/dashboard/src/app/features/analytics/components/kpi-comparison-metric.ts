import type { ComparisonPeriod } from '@core/analytics/comparison-period';
import { calcDelta } from '@core/analytics/delta-utils';
import type { KpiCardModel } from './kpi-card';

interface KpiBaseline {
    period: ComparisonPeriod;
    value: number;
}

export interface KpiComparisonCard extends KpiCardModel {
    id: string;
    description?: string;
    /** Omit for live metrics. A null baseline still allows a comparison to be inspected. */
    previous?: KpiBaseline | null;
}

export const KPI_DELTA_FORMAT: Intl.NumberFormatOptions = Object.freeze({
    style: 'percent',
    signDisplay: 'exceptZero',
    minimumFractionDigits: 1,
    maximumFractionDigits: 1
});

export function kpiBaseline(period: ComparisonPeriod | null, value: number | null | undefined): KpiBaseline | null {
    return period && value != null && Number.isFinite(value) ? { period, value } : null;
}

export function kpiDelta(card: KpiComparisonCard): number | null {
    return card.previous && typeof card.value === 'number' && Number.isFinite(card.value) ? calcDelta(card.value, card.previous.value) : null;
}

export function kpiDeltaClass(delta: number | null, invert: boolean): string {
    if (delta === null || !Number.isFinite(delta)) return '';
    if (delta === 0) return 'text-xs font-medium px-1.5 py-0.5 rounded-full bg-surface-100 dark:bg-surface-800 text-muted-color';
    return (invert ? delta < 0 : delta > 0)
        ? 'text-xs font-medium px-1.5 py-0.5 rounded-full bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
        : 'text-xs font-medium px-1.5 py-0.5 rounded-full bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400';
}
