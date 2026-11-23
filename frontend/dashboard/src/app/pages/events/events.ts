import { ChangeDetectionStrategy, Component, computed, DestroyRef, effect, inject, signal, untracked } from '@angular/core';
import { FormsModule, ReactiveFormsModule } from '@angular/forms';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { SelectModule } from '@openng/optimus-ui/select';
import { CardModule } from '@openng/optimus-ui/card';
import { SkeletonModule } from '@openng/optimus-ui/skeleton';
import { ButtonModule } from '@openng/optimus-ui/button';
import { MessageModule } from '@openng/optimus-ui/message';
import { SiteService } from '@features/sites/services/site.service';
import { toggleDimensionFilter } from '@core/analytics/filter-utils';
import { AnalyticsService, EventDimensionFilter } from '@core/services/analytics.service';
import { MetricCardConfig, MetricCardGroup, MetricCardGroupRowClick, MetricCardGroupTab } from '@features/analytics/components/metric-card-group';
import { ReportRangeToolbar } from '@components/report-range-toolbar/report-range-toolbar';
import { SeriesChart, SeriesDefinition, SeriesChartPoint } from '@features/analytics/components/series-chart';
import { MetricStat, EventSeriesPoint, EventAudience } from '@models/analytics.types';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { catchError, EMPTY, finalize, forkJoin, of, Subscription } from 'rxjs';
import { injectActiveLang } from '@core/i18n/active-lang';
import { isIntradayPeriod, previousPeriod, type ComparisonPeriod } from '@core/analytics/comparison-period';
import { KpiComparisonDialog } from '@features/analytics/components/kpi-comparison-dialog';
import { KpiComparisonTrigger } from '@features/analytics/components/kpi-comparison-trigger';
import { kpiBaseline, type KpiComparisonCard, kpiDelta } from '@features/analytics/components/kpi-comparison-metric';
import { RealtimeRefreshCoordinator } from '@services/realtime-refresh-coordinator.service';
import { REALTIME_EVENT_KINDS } from '@services/realtime.service';
import { injectReportRange } from '@services/report-range-preferences.service';
import { injectSkeletonGate } from '@services/report-subject.service';
import { SetupStateService } from '@services/setup-state.service';
import { AnimatedNumber } from '@components/animated-number/animated-number';
import { NoSiteSelected } from '@components/no-site-selected/no-site-selected';
import { SetupCallout } from '@components/setup-callout/setup-callout';

interface EventFilterChip {
    key: string;
    label: string;
    remove: () => void;
}

type EventDimensionFilterType = 'path' | 'referrer' | 'device' | 'country' | 'city' | 'provider' | 'asn';
type EventMetricFilterType = 'propertyValue' | EventDimensionFilterType;

interface EventOption {
    label: string;
    value: string;
    isAutomatic: boolean;
    icon: string;
}

const AUTOMATIC_EVENT_META: Record<string, { icon: string }> = {
    outbound_click: { icon: 'pi pi-external-link' },
    file_download: { icon: 'pi pi-download' },
    form_submit: { icon: 'pi pi-send' }
};
const AUTOMATIC_EVENT_NAMES = Object.keys(AUTOMATIC_EVENT_META);

@Component({
    selector: 'app-events',
    imports: [
        FormsModule,
        ReactiveFormsModule,
        TranslocoPipe,
        SelectModule,
        CardModule,
        SkeletonModule,
        ButtonModule,
        MessageModule,
        MetricCardGroup,
        ReportRangeToolbar,
        SeriesChart,
        AnimatedNumber,
        KpiComparisonDialog,
        KpiComparisonTrigger,
        NoSiteSelected,
        SetupCallout
    ],
    templateUrl: './events.html',
    styleUrl: './events.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class Events {
    protected readonly addEventsDocsUrl = 'https://hitkeep.com/guides/tracking/automatic-events/';
    protected readonly customEventsDocsUrl = 'https://hitkeep.com/guides/tracking/custom-events/';
    private siteService = inject(SiteService);
    private analyticsService = inject(AnalyticsService);
    private transloco = inject(TranslocoService);
    private destroyRef = inject(DestroyRef);
    private realtimeRefresh = inject(RealtimeRefreshCoordinator);
    private readonly setupState = inject(SetupStateService);
    private readonly reportRange = injectReportRange();
    private readonly activeLanguage = injectActiveLang();

    protected eventNames = signal<string[]>([]);
    /** Names the site actually reported, before the automatic ones get merged in; `null` until the first answer. */
    private readonly reportedEventNames = signal<string[] | null>(null);
    private realtimeRefreshKey = signal(0);
    protected selectedEvent = signal<string | null>(null);
    protected propertyKeys = signal<string[]>([]);
    protected selectedPropertyKey = signal<string | null>(null);
    protected selectedPropertyValue = signal<string | null>(null);
    protected breakdown = signal<MetricStat[]>([]);
    protected eventSeries = signal<EventSeriesPoint[]>([]);
    protected comparisonEventSeries = signal<EventSeriesPoint[]>([]);
    protected audience = signal<EventAudience | null>(null);
    protected audienceDimFilters = signal<EventDimensionFilter[]>([]);

    protected isLoadingNames = signal(false);
    protected isLoadingKeys = signal(false);
    protected isLoadingBreakdown = signal(false);
    protected isLoadingEventSeries = signal(false);
    protected isLoadingAudience = signal(false);

    private seriesRequest?: Subscription;
    protected readonly seriesRange = signal<ComparisonPeriod | null>(null);
    protected readonly comparisonRange = signal<ComparisonPeriod | null>(null);
    protected readonly isShortRange = computed(() => isIntradayPeriod(this.seriesRange()));

    protected readonly eventSeriesChart = computed<SeriesChartPoint[]>(() => this.eventSeries().map((p) => ({ time: p.time, count: p.count })));
    protected readonly comparisonEventSeriesChart = computed<SeriesChartPoint[]>(() => this.comparisonEventSeries().map((p) => ({ time: p.time, count: p.count })));
    protected readonly totalEventCount = computed(() => this.eventSeries().reduce((sum, p) => sum + p.count, 0));
    protected readonly comparisonTotalCount = computed(() => this.comparisonEventSeries().reduce((sum, p) => sum + p.count, 0));
    protected readonly totalEventKpi = computed<KpiComparisonCard>(() => {
        this.activeLanguage();
        return {
            id: 'total_events',
            label: this.transloco.translate('events.kpis.totalEvents'),
            value: this.totalEventCount(),
            loading: this.isLoadingEventSeries(),
            previous: kpiBaseline(this.comparisonRange(), this.comparisonTotalCount())
        };
    });
    protected readonly totalEventDelta = computed(() => kpiDelta(this.totalEventKpi()));
    protected readonly isLoading = computed(() => this.isLoadingNames() || this.isLoadingEventSeries());
    protected readonly showEventTotalSkeleton = injectSkeletonGate(this.isLoadingEventSeries, () => this.eventSeries().length > 0);

    protected readonly activeSite = computed(() => this.siteService.activeSite());
    protected readonly noSite = computed(() => !this.activeSite());

    /**
     * True only once the shared setup state confirms the site never sent a
     * custom event. Empty ranges on an instrumented site keep the regular empty
     * states, and the automatic events stay reachable there.
     */
    protected readonly needsSetup = computed(() => {
        const reported = this.reportedEventNames();
        return this.setupState.needsSetup(this.activeSite()?.id, 'has_custom_events', reported ? reported.length : null, this.isLoadingNames());
    });
    protected eventOptions = computed<EventOption[]>(() =>
        this.eventNames().map((name) => {
            const meta = AUTOMATIC_EVENT_META[name];
            return {
                label: name,
                value: name,
                isAutomatic: !!meta,
                icon: meta?.icon ?? ''
            };
        })
    );
    protected propertyOptions = computed(() => this.propertyKeys().map((key) => ({ label: key, value: key })));

    protected readonly eventSeriesConfig = computed<SeriesDefinition[]>(() => {
        this.activeLanguage();
        return [
            {
                key: 'count',
                label: this.transloco.translate('events.kpis.totalEvents'),
                color: '#6366f1',
                gradientFrom: 'rgba(99, 102, 241, 0.5)',
                gradientTo: 'rgba(99, 102, 241, 0.0)'
            }
        ];
    });

    // Unified filter chips — property value + audience dimension filter
    protected readonly filterChips = computed<EventFilterChip[]>(() => {
        this.activeLanguage();
        const chips: EventFilterChip[] = [];
        const propKey = this.selectedPropertyKey();
        const propValue = this.selectedPropertyValue();
        if (propKey && propValue) {
            chips.push({ key: `property:${propKey}:${propValue}`, label: `${propKey}: ${propValue}`, remove: () => this.selectedPropertyValue.set(null) });
        }
        for (const dim of this.audienceDimFilters()) {
            chips.push({
                key: `dimension:${dim.type}:${dim.value}`,
                label: this.dimFilterLabel(dim.type, dim.value),
                remove: () => this.removeDimensionFilter(dim.type, dim.value)
            });
        }
        return chips;
    });

    protected readonly hasActiveFilters = computed(() => this.filterChips().length > 0);

    protected readonly metricCardTabs = computed<MetricCardGroupTab<EventMetricFilterType>[]>(() => {
        this.activeLanguage();
        const audience = this.audience();
        const loading = this.isLoadingAudience();
        const contentCards: MetricCardConfig<EventMetricFilterType>[] = [];
        if (this.selectedEvent() && this.selectedPropertyKey()) {
            contentCards.push({
                id: 'property-breakdown',
                title: this.transloco.translate('events.breakdown.title'),
                icon: 'pi-list',
                data: this.breakdown(),
                isLoading: this.isLoadingBreakdown(),
                isRowClickable: true,
                activeValue: this.selectedPropertyValue(),
                filterType: 'propertyValue'
            });
        }
        if (this.selectedEvent()) {
            contentCards.push({
                id: 'top-pages',
                title: this.transloco.translate('common.metrics.topPages'),
                icon: 'pi-file',
                data: audience?.top_pages ?? [],
                linkMode: 'path',
                siteDomain: this.activeSite()?.domain ?? null,
                isLoading: loading,
                isRowClickable: true,
                activeValue: this.activeDimensionFilterValue('path'),
                filterType: 'path'
            });
        }
        return [
            {
                id: 'content',
                label: this.transloco.translate('common.metricGroups.content'),
                icon: 'pi-file',
                cards: contentCards
            },
            {
                id: 'acquisition',
                label: this.transloco.translate('common.metricGroups.acquisition'),
                icon: 'pi-link',
                cards: [
                    {
                        id: 'sources',
                        title: this.transloco.translate('common.metrics.topSources'),
                        icon: 'pi-link',
                        data: audience?.top_referrers ?? [],
                        linkMode: 'url',
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeDimensionFilterValue('referrer'),
                        filterType: 'referrer'
                    }
                ]
            },
            {
                id: 'audience',
                label: this.transloco.translate('common.metricGroups.audience'),
                icon: 'pi-users',
                cards: [
                    {
                        id: 'devices',
                        title: this.transloco.translate('common.metrics.devices'),
                        icon: 'pi-mobile',
                        data: audience?.top_devices ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeDimensionFilterValue('device'),
                        filterType: 'device'
                    }
                ]
            },
            {
                id: 'location',
                label: this.transloco.translate('common.metricGroups.location'),
                icon: 'pi-map',
                cards: [
                    {
                        id: 'countries',
                        title: this.transloco.translate('common.metrics.countries'),
                        icon: 'pi-globe',
                        data: audience?.top_countries ?? [],
                        isLoading: loading,
                        showCountryFlags: true,
                        showCountryNames: true,
                        isRowClickable: true,
                        activeValue: this.activeDimensionFilterValue('country'),
                        filterType: 'country'
                    },
                    {
                        id: 'cities',
                        title: this.transloco.translate('common.metrics.cities'),
                        icon: 'pi-map-marker',
                        data: audience?.top_cities ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeDimensionFilterValue('city'),
                        filterType: 'city'
                    }
                ]
            },
            {
                id: 'network',
                label: this.transloco.translate('common.metricGroups.network'),
                icon: 'pi-server',
                cards: [
                    {
                        id: 'providers',
                        title: this.transloco.translate('common.metrics.providers'),
                        icon: 'pi-server',
                        data: audience?.top_providers ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeDimensionFilterValue('provider'),
                        filterType: 'provider'
                    },
                    {
                        id: 'asns',
                        title: this.transloco.translate('common.metrics.asns'),
                        icon: 'pi-sitemap',
                        data: audience?.top_asns ?? [],
                        isLoading: loading,
                        isRowClickable: true,
                        activeValue: this.activeDimensionFilterValue('asn'),
                        filterType: 'asn'
                    }
                ]
            }
        ];
    });

    constructor() {
        // Load event names when site/range changes; reconcile selection
        effect((onCleanup) => {
            const site = this.activeSite();
            this.reportRange.selectedRange();
            this.realtimeRefreshKey();
            if (!site) return;
            const dates = this.getCurrentDateRange();
            if (!dates) return;
            const request = this.loadEventNames(site.id, dates.from, dates.to);
            onCleanup(() => request.unsubscribe());
        });

        // Load property keys when selected event or range changes; reconcile selection
        effect((onCleanup) => {
            const site = this.activeSite();
            const eventName = this.selectedEvent();
            this.reportRange.selectedRange();
            this.realtimeRefreshKey();
            if (!site || !eventName) {
                this.propertyKeys.set([]);
                untracked(() => {
                    this.selectedPropertyKey.set(null);
                    this.selectedPropertyValue.set(null);
                });
                return;
            }
            const dates = this.getCurrentDateRange();
            if (!dates) return;
            const request = this.loadPropertyKeys(site.id, dates.from, dates.to, eventName);
            onCleanup(() => request.unsubscribe());
        });

        // Reset property value filter when property key changes
        effect(() => {
            this.selectedPropertyKey();
            untracked(() => this.selectedPropertyValue.set(null));
        });

        // Reset audience dimension filter when event changes
        effect(() => {
            this.selectedEvent();
            untracked(() => this.audienceDimFilters.set([]));
        });

        // Load event timeseries (primary + comparison) when event/range/propertyFilter/dimFilter changes
        effect(() => {
            const site = this.activeSite();
            const eventName = this.selectedEvent();
            const propKey = this.selectedPropertyKey();
            const propValue = this.selectedPropertyValue();
            const dimFilters = this.audienceDimFilters();
            this.reportRange.selectedRange();
            this.realtimeRefreshKey();

            if (!site || !eventName) {
                this.seriesRequest?.unsubscribe();
                this.eventSeries.set([]);
                this.comparisonEventSeries.set([]);
                this.seriesRange.set(null);
                this.comparisonRange.set(null);
                return;
            }
            const dates = this.getCurrentDateRange();
            if (!dates) return;

            const filterKey = propKey && propValue ? propKey : undefined;
            const filterVal = propKey && propValue ? propValue : undefined;
            this.loadEventTimeseries(site.id, dates.from, dates.to, eventName, filterKey, filterVal, dimFilters);
        });

        // Load breakdown when event/propertyKey/range changes
        effect((onCleanup) => {
            const site = this.activeSite();
            const eventName = this.selectedEvent();
            const propertyKey = this.selectedPropertyKey();
            this.reportRange.selectedRange();
            this.realtimeRefreshKey();
            if (!site || !eventName || !propertyKey) {
                this.breakdown.set([]);
                return;
            }
            const dates = this.getCurrentDateRange();
            if (!dates) return;
            const request = this.loadBreakdown(site.id, dates.from, dates.to, eventName, propertyKey);
            onCleanup(() => request.unsubscribe());
        });

        // Load audience panels when event/property filter/dimension filter/range changes
        effect((onCleanup) => {
            const site = this.activeSite();
            const eventName = this.selectedEvent();
            const propKey = this.selectedPropertyKey();
            const propValue = this.selectedPropertyValue();
            const dimFilters = this.audienceDimFilters();
            this.reportRange.selectedRange();
            this.realtimeRefreshKey();

            if (!site || !eventName) {
                this.audience.set(null);
                return;
            }
            const dates = this.getCurrentDateRange();
            if (!dates) return;

            const filterKey = propKey && propValue ? propKey : undefined;
            const filterVal = propKey && propValue ? propValue : undefined;
            const request = this.loadAudience(site.id, dates.from, dates.to, eventName, filterKey, filterVal, dimFilters);
            onCleanup(() => request.unsubscribe());
        });
        this.realtimeRefresh.registerSignalUntilDestroyed(this.destroyRef, {
            siteId: () => this.activeSite()?.id ?? null,
            kinds: REALTIME_EVENT_KINDS,
            enabled: () => !!this.activeSite() && !!this.getCurrentDateRange(),
            signal: this.realtimeRefreshKey,
            debounceMs: 700
        });
    }

    protected clearAllFilters() {
        this.selectedPropertyValue.set(null);
        this.audienceDimFilters.set([]);
    }

    protected togglePropertyValueFilter(value: string) {
        const current = this.selectedPropertyValue();
        this.selectedPropertyValue.set(current === value ? null : value);
    }

    protected toggleAudienceDimFilter(type: EventDimensionFilterType, item: MetricStat) {
        this.audienceDimFilters.update((filters) => toggleDimensionFilter(filters, type, item.name));
    }

    protected onMetricCardClick(event: MetricCardGroupRowClick): void {
        if (event.filterType === 'propertyValue') {
            this.togglePropertyValueFilter(event.metric.name);
            return;
        }
        if (!this.isEventDimensionFilterType(event.filterType)) return;
        this.toggleAudienceDimFilter(event.filterType, event.metric);
    }

    protected activeDimensionFilterValue(type: EventDimensionFilterType): string | null {
        return this.audienceDimFilters().find((filter) => filter.type === type)?.value ?? null;
    }

    private removeDimensionFilter(type: string, value: string) {
        this.audienceDimFilters.update((filters) => filters.filter((filter) => !(filter.type === type && filter.value === value)));
    }

    private dimFilterLabel(dim: string, value: string): string {
        switch (dim) {
            case 'path':
                return this.transloco.translate('common.filters.page', { value });
            case 'referrer':
                return this.transloco.translate('common.filters.source', { value });
            case 'device':
                return this.transloco.translate('common.filters.device', { value });
            case 'country':
                return this.transloco.translate('common.filters.country', { value });
            case 'city':
                return this.transloco.translate('common.filters.city', { value });
            case 'provider':
                return this.transloco.translate('common.filters.provider', { value });
            case 'asn':
                return this.transloco.translate('common.filters.asn', { value });
            default:
                return `${dim}: ${value}`;
        }
    }

    private isEventDimensionFilterType(value: string): value is EventDimensionFilterType {
        return value === 'path' || value === 'referrer' || value === 'device' || value === 'country' || value === 'city' || value === 'provider' || value === 'asn';
    }

    protected getCurrentDateRange(): { from: string; to: string } | null {
        return this.reportRange.currentDateRange();
    }

    private loadEventNames(siteId: string, from: string, to: string) {
        this.isLoadingNames.set(true);
        return this.analyticsService
            .getEventNames(siteId, from, to)
            .pipe(
                catchError(() => EMPTY),
                finalize(() => this.isLoadingNames.set(false))
            )
            .subscribe((names) => {
                this.reportedEventNames.set(names);
                const mergedNames = this.mergeAutomaticEventNames(names);
                this.eventNames.set(mergedNames);
                const currentSelection = untracked(this.selectedEvent);
                this.selectedEvent.set(currentSelection && mergedNames.includes(currentSelection) ? currentSelection : null);
            });
    }

    private mergeAutomaticEventNames(names: string[]): string[] {
        const automaticNames = AUTOMATIC_EVENT_NAMES.filter((name) => !names.includes(name));
        return [...automaticNames, ...names];
    }

    private loadPropertyKeys(siteId: string, from: string, to: string, eventName: string) {
        this.isLoadingKeys.set(true);
        return this.analyticsService
            .getEventPropertyKeys(siteId, from, to, eventName)
            .pipe(
                catchError(() => EMPTY),
                finalize(() => this.isLoadingKeys.set(false))
            )
            .subscribe((keys) => {
                this.propertyKeys.set(keys);
                const currentPropertyKey = untracked(this.selectedPropertyKey);
                this.selectedPropertyKey.set(currentPropertyKey && keys.includes(currentPropertyKey) ? currentPropertyKey : null);
            });
    }

    private loadBreakdown(siteId: string, from: string, to: string, eventName: string, propertyKey: string) {
        this.isLoadingBreakdown.set(true);
        return this.analyticsService
            .getEventPropertyBreakdown(siteId, from, to, eventName, propertyKey)
            .pipe(
                catchError(() => EMPTY),
                finalize(() => this.isLoadingBreakdown.set(false))
            )
            .subscribe((data) => this.breakdown.set(data));
    }

    private loadEventTimeseries(siteId: string, from: string, to: string, eventName: string, propertyKey?: string, propertyValue?: string, filters: EventDimensionFilter[] = []) {
        this.seriesRequest?.unsubscribe();
        this.isLoadingEventSeries.set(true);
        const comparison = previousPeriod(from, to);
        this.seriesRequest = forkJoin({
            current: this.analyticsService.getEventTimeseries(siteId, from, to, eventName, propertyKey, propertyValue, filters),
            previous: this.analyticsService.getEventTimeseries(siteId, comparison.from, comparison.to, eventName, propertyKey, propertyValue, filters).pipe(catchError(() => of(null)))
        })
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.isLoadingEventSeries.set(false))
            )
            .subscribe({
                next: ({ current, previous }) => {
                    this.eventSeries.set(current ?? []);
                    this.comparisonEventSeries.set(previous ?? []);
                    this.seriesRange.set({ from, to });
                    this.comparisonRange.set(previous ? comparison : null);
                },
                error: () => {
                    this.eventSeries.set([]);
                    this.comparisonEventSeries.set([]);
                    this.seriesRange.set(null);
                    this.comparisonRange.set(null);
                }
            });
    }

    private loadAudience(siteId: string, from: string, to: string, eventName: string, propertyKey?: string, propertyValue?: string, filters: EventDimensionFilter[] = []) {
        this.isLoadingAudience.set(true);
        return this.analyticsService
            .getEventAudience(siteId, from, to, eventName, propertyKey, propertyValue, filters)
            .pipe(finalize(() => this.isLoadingAudience.set(false)))
            .subscribe({
                next: (data) => this.audience.set(data),
                error: () => this.audience.set(null)
            });
    }
}
