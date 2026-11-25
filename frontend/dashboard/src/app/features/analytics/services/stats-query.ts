import { computed, inject, Injector, signal, untracked } from '@angular/core';
import { rxResource } from '@angular/core/rxjs-interop';
import { finalize, tap } from 'rxjs';
import { previousPeriod, type ComparisonPeriod } from '@core/analytics/comparison-period';
import type { SiteStats } from '@models/analytics.types';
import { StatsService } from '@features/analytics/services/stats.service';

export interface StatsQueryRequest {
    siteId: string;
    from: string;
    to: string;
    filters?: { type: string; value: string }[];
    goalIds?: string[];
    funnelIds?: string[];
    view?: 'utm';
    mode?: StatsQueryMode;
    onSuccess?: (stats: SiteStats, result: StatsQueryResult) => void;
}

export type StatsQueryMode = 'blocking' | 'background';

export interface StatsQueryResult {
    mode: StatsQueryMode;
    sequence: number;
}

export class StatsQuery {
    private readonly loaded = signal<{ siteId: string; stats: SiteStats; period: ComparisonPeriod; comparison: ComparisonPeriod | null } | null>(null);
    readonly stats = computed(() => this.loaded()?.stats ?? null);
    readonly currentRange = computed(() => this.loaded()?.period ?? null);
    readonly comparisonRange = computed(() => this.loaded()?.comparison ?? null);
    readonly isLoading = signal(false);
    readonly isBackgroundRefreshing = signal(false);
    readonly blockingFailed = signal(false);
    readonly lastResult = signal<StatsQueryResult | null>(null);

    private readonly request = signal<StatsQueryRequest | undefined>(undefined);
    private readonly query;
    private resultSequence = 0;

    constructor(
        private readonly statsService: StatsService,
        injector: Injector
    ) {
        this.query = rxResource({
            params: () => this.request(),
            stream: ({ params }) =>
                this.statsService.fetchStats(params.siteId, params.from, params.to, params.filters ?? [], params.goalIds ?? [], params.funnelIds ?? [], params.view).pipe(
                    tap({
                        next: (stats) => {
                            if (this.request() !== params) return;
                            const result = { mode: params.mode ?? 'blocking', sequence: ++this.resultSequence };
                            const period = { from: params.from, to: params.to };
                            this.blockingFailed.set(false);
                            this.loaded.set({ siteId: params.siteId, stats, period, comparison: stats.comparison ? previousPeriod(period.from, period.to) : null });
                            this.lastResult.set(result);
                            params.onSuccess?.(stats, result);
                        },
                        error: (error) => {
                            if (this.request() === params && params.mode === 'blocking') this.blockingFailed.set(true);
                            console.error(error);
                        }
                    }),
                    finalize(() => {
                        if (this.request() !== params) return;
                        if ((params.mode ?? 'blocking') === 'background') {
                            this.isBackgroundRefreshing.set(false);
                        } else {
                            this.isLoading.set(false);
                        }
                    })
                ),
            injector
        });
    }

    load(request: StatsQueryRequest): void {
        const loaded = untracked(this.loaded);
        if (loaded && loaded.siteId !== request.siteId) this.loaded.set(null);
        const mode = request.mode ?? 'blocking';
        if (mode === 'background') {
            this.isLoading.set(false);
            this.isBackgroundRefreshing.set(true);
        } else {
            this.isBackgroundRefreshing.set(false);
            this.isLoading.set(true);
        }
        this.request.set({ ...request, mode });
    }
}

export function injectStatsQuery(): StatsQuery {
    return new StatsQuery(inject(StatsService), inject(Injector));
}
