import { ChangeDetectionStrategy, Component, computed, DestroyRef, effect, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormControl } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoDecimalPipe } from '@jsverse/transloco-locale';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { ButtonModule } from '@openng/optimus-ui/button';
import { CardModule } from '@openng/optimus-ui/card';
import { ConfirmDialogModule } from '@openng/optimus-ui/confirmdialog';
import { TagModule } from '@openng/optimus-ui/tag';
import { catchError, finalize, forkJoin, of, Subscription } from 'rxjs';
import { SITE_CAPABILITIES } from '@core/access/capabilities';
import { isIntradayPeriod, previousPeriod, type ComparisonPeriod } from '@core/analytics/comparison-period';
import { HITKEEP_CHART_PALETTE } from '@core/charts/hitkeep-chart-options';
import { injectActiveLang } from '@core/i18n/active-lang';
import { dialogCancelButton, dialogDangerButton } from '@components/dialog-actions/dialog-actions';
import { PageState } from '@components/page-state/page-state';
import { ReportRangeToolbar } from '@components/report-range-toolbar/report-range-toolbar';
import { AppTable, AppTableCell, AppTableColumn } from '@components/table/table';
import { TableRowActionItem } from '@components/table-row-actions/table-row-actions';
import { KPI_PERCENT_FORMAT } from '@features/analytics/components/kpi-card';
import { KpiComparisonCards } from '@features/analytics/components/kpi-comparison-cards';
import { kpiBaseline, type KpiComparisonCard } from '@features/analytics/components/kpi-comparison-metric';
import { injectConversionDefinitions } from '@features/analytics/services/conversion-definitions';
import { ConversionSubjectCard } from '@features/analytics/components/conversion-subject-card';
import { SeriesChart, SeriesChartPoint, SeriesDefinition } from '@features/analytics/components/series-chart';
import { TrafficRecordsCard } from '@features/hits/components/traffic-records-card';
import { injectStatsQuery, StatsQueryMode } from '@features/analytics/services/stats-query';
import { FunnelManager } from '@features/funnels/components/funnel-manager';
import { SiteService } from '@features/sites/services/site.service';
import { Funnel, FunnelSeriesPoint, FunnelStats } from '@models/analytics.types';
import { AccessService } from '@services/access.service';
import { AnalyticsService } from '@services/analytics.service';
import { NavigationNoticeService } from '@services/navigation-notice.service';
import { REALTIME_FUNNEL_KINDS } from '@services/realtime.service';
import { RealtimeRefreshCoordinator } from '@services/realtime-refresh-coordinator.service';
import { injectReportRange } from '@services/report-range-preferences.service';
import { ShareService } from '@services/share.service';
import { EmbedService } from '@services/embed.service';

@Component({
    selector: 'app-funnels',
    standalone: true,
    imports: [
        ButtonModule,
        CardModule,
        ConfirmDialogModule,
        ConversionSubjectCard,
        FunnelManager,
        KpiComparisonCards,
        PageState,
        ReportRangeToolbar,
        SeriesChart,
        AppTable,
        AppTableCell,
        TagModule,
        TrafficRecordsCard,
        TranslocoDecimalPipe,
        TranslocoPipe
    ],
    providers: [ConfirmationService],
    templateUrl: './funnels.html',
    styleUrl: './funnels.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class Funnels {
    protected readonly embed = inject(EmbedService);
    protected siteService = inject(SiteService);
    private analytics = inject(AnalyticsService);
    private access = inject(AccessService);
    private route = inject(ActivatedRoute);
    private router = inject(Router);
    private transloco = inject(TranslocoService);
    private confirmation = inject(ConfirmationService);
    private destroyRef = inject(DestroyRef);
    private realtime = inject(RealtimeRefreshCoordinator);
    private notice = inject(NavigationNoticeService);
    private share = inject(ShareService);
    protected reportRange = injectReportRange();
    private activeLanguage = injectActiveLang();
    private statsQuery = injectStatsQuery();
    private lastSiteId: string | null = null;
    private createRequested = false;
    private reportingRequest: Subscription | null = null;
    private funnelStatsRequest: Subscription | null = null;
    private reportingSequence = 0;
    private funnelStatsSequence = 0;

    private readonly definitions = injectConversionDefinitions((siteId) => this.analytics.getFunnels(siteId));
    protected readonly funnels = this.definitions.items;
    protected readonly loading = this.definitions.loading;
    protected readonly loadError = this.definitions.error;
    protected selectedFunnelId = signal<string | null>(null);
    protected selectedFunnel = computed(() => this.funnels().find((funnel) => funnel.id === this.selectedFunnelId()) ?? null);
    protected subjectControl = new FormControl<string | null>(null);
    protected editorVisible = signal(false);
    protected editingFunnel = signal<Funnel | null>(null);
    protected deletingFunnelId = signal<string | null>(null);
    protected readonly funnelRows = computed(() => this.funnels().map((funnel) => ({ ...funnel, step_path: funnel.steps.map((step) => step.value).join(' → ') })));
    protected readonly funnelColumns: AppTableColumn<Funnel & { step_path: string }>[] = [
        { field: 'name', headerKey: 'common.columns.name', frozen: true },
        { field: 'step_path', headerKey: 'funnels.definitions.steps', sortable: false },
        { field: 'created_at', headerKey: 'common.columns.created', type: 'date', hidden: true }
    ];
    protected readonly isSelectedFunnel = (funnel: Funnel) => funnel.id === this.selectedFunnelId();
    protected readonly funnelActionLoading = (funnel: Funnel) => this.deletingFunnelId() === funnel.id;
    protected funnelSeries = signal<FunnelSeriesPoint[]>([]);
    protected comparisonFunnelSeries = signal<FunnelSeriesPoint[]>([]);
    protected seriesLoading = signal(false);
    protected funnelStats = signal<FunnelStats | null>(null);
    protected funnelStatsLoading = signal(false);
    protected trafficRefreshKey = signal(0);
    protected readonly seriesRange = signal<ComparisonPeriod | null>(null);
    protected readonly comparisonRange = signal<ComparisonPeriod | null>(null);
    protected readonly isShortRange = computed(() => isIntradayPeriod(this.seriesRange()));
    protected stats = this.statsQuery.stats;
    protected isStatsLoading = this.statsQuery.isLoading;
    protected isRefreshing = computed(() => this.loading() || this.seriesLoading() || this.funnelStatsLoading());
    protected canManage = computed(() => {
        const site = this.siteService.activeSite();
        return !!site && this.access.canSite(site.id, SITE_CAPABILITIES.manageGoals);
    });
    protected trafficFunnelIds = computed(() => {
        const selected = this.selectedFunnelId();
        return this.funnels()
            .filter((funnel) => !selected || funnel.id === selected)
            .map((funnel) => funnel.id);
    });
    protected trafficEnabled = computed(() => this.trafficFunnelIds().length > 0);
    protected trafficDescriptionKey = computed(() => (this.selectedFunnelId() ? 'funnels.traffic.selectedDescription' : 'funnels.traffic.allDescription'));
    protected trafficShareToken = computed(() => this.share.token());
    protected subjectOptions = computed(() => {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('funnels.subject.all'), value: null },
            ...this.funnels().map((funnel) => ({
                label: funnel.name,
                value: funnel.id
            }))
        ];
    });
    protected subjectDescription = computed(() => {
        this.activeLanguage();
        const funnel = this.selectedFunnel();
        if (!funnel)
            return this.transloco.translate('funnels.definitions.count', {
                count: this.funnels().length
            });
        const steps = funnel.steps.map((step) => step.value).join(' → ');
        return `${this.transloco.translate('funnels.list.stepsCount', { count: funnel.steps.length })} · ${steps}`;
    });
    protected seriesData = computed<SeriesChartPoint[]>(() =>
        this.funnelSeries().map((point) => ({
            time: point.time,
            entries: point.entries,
            completions: point.completions
        }))
    );
    protected comparisonSeriesData = computed<SeriesChartPoint[]>(() =>
        this.comparisonFunnelSeries().map((point) => ({
            time: point.time,
            entries: point.entries,
            completions: point.completions
        }))
    );
    protected seriesConfig = computed<SeriesDefinition[]>(() => {
        this.activeLanguage();
        return [
            {
                key: 'entries',
                label: this.transloco.translate('funnels.kpis.entries'),
                color: HITKEEP_CHART_PALETTE.primary
            },
            {
                key: 'completions',
                label: this.transloco.translate('funnels.kpis.completions'),
                color: HITKEEP_CHART_PALETTE.secondary
            }
        ];
    });
    protected kpis = computed<KpiComparisonCard[]>(() => {
        this.activeLanguage();
        const entries = this.funnelSeries().reduce((sum, point) => sum + point.entries, 0);
        const completions = this.funnelSeries().reduce((sum, point) => sum + point.completions, 0);
        const comparisonEntries = this.comparisonFunnelSeries().reduce((sum, point) => sum + point.entries, 0);
        const comparisonCompletions = this.comparisonFunnelSeries().reduce((sum, point) => sum + point.completions, 0);
        const rate = entries ? (completions / entries) * 100 : 0;
        const comparisonRate = comparisonEntries ? (comparisonCompletions / comparisonEntries) * 100 : 0;
        const period = this.comparisonRange();
        return [
            {
                id: 'entries',
                label: this.transloco.translate('funnels.kpis.entries'),
                value: entries,
                loading: this.seriesLoading(),
                previous: kpiBaseline(period, comparisonEntries)
            },
            {
                id: 'completions',
                label: this.transloco.translate('funnels.kpis.completions'),
                value: completions,
                loading: this.seriesLoading(),
                previous: kpiBaseline(period, comparisonCompletions)
            },
            {
                id: 'completion_rate',
                label: this.transloco.translate('funnels.kpis.completionRate'),
                value: rate,
                loading: this.seriesLoading(),
                format: KPI_PERCENT_FORMAT,
                suffix: '%',
                previous: kpiBaseline(period, comparisonRate)
            }
        ];
    });
    protected pathSuggestions = computed(() => (this.stats()?.top_pages ?? []).map((item) => item.name).filter(Boolean));
    protected eventSuggestions = computed(() => [...new Set(this.funnels().flatMap((funnel) => funnel.steps.filter((step) => step.type === 'event').map((step) => step.value)))]);

    constructor() {
        this.destroyRef.onDestroy(() => {
            this.reportingRequest?.unsubscribe();
            this.funnelStatsRequest?.unsubscribe();
        });
        this.route.queryParamMap.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((params) => {
            const id = params.get('funnel');
            this.selectedFunnelId.set(id);
            this.subjectControl.setValue(id, { emitEvent: false });
            this.createRequested = params.get('create') === '1';
        });
        effect(() => {
            const site = this.siteService.activeSite();
            if (this.lastSiteId && site?.id !== this.lastSiteId) {
                this.selectedFunnelId.set(null);
                this.subjectControl.setValue(null, { emitEvent: false });
                void this.router.navigate([], {
                    relativeTo: this.route,
                    queryParams: { funnel: null },
                    queryParamsHandling: 'merge',
                    replaceUrl: true
                });
            }
            this.lastSiteId = site?.id ?? null;
        });
        effect(() => {
            const site = this.siteService.activeSite();
            const dates = this.reportRange.currentDateRange();
            const funnels = this.funnels();
            const selected = this.selectedFunnelId();
            if (!site || !dates || this.loading() || this.loadError()) return;
            if (this.createRequested && this.canManage()) {
                this.createRequested = false;
                this.openCreate();
            }
            if (selected && !funnels.some((funnel) => funnel.id === selected)) {
                this.notice.show('funnels.notices.unavailable', {
                    preserveNextNavigation: true
                });
                this.selectFunnel(null);
                return;
            }
            this.loadReporting(site.id, dates.from, dates.to, selected ? [selected] : []);
            if (selected) this.loadFunnelStats(site.id, selected, dates.from, dates.to);
            else this.clearFunnelStats();
        });
        this.realtime.registerUntilDestroyed(this.destroyRef, {
            siteId: () => this.siteService.activeSite()?.id ?? null,
            kinds: REALTIME_FUNNEL_KINDS,
            enabled: () => !!this.siteService.activeSite(),
            refresh: () => this.refresh('background'),
            debounceMs: 700
        });
    }

    protected selectFunnel(id: string | null) {
        this.selectedFunnelId.set(id);
        this.subjectControl.setValue(id, { emitEvent: false });
        void this.router.navigate([], {
            relativeTo: this.route,
            queryParams: { funnel: id, create: null },
            queryParamsHandling: 'merge',
            replaceUrl: true
        });
    }
    protected openCreate() {
        if (!this.canManage()) return;
        this.editingFunnel.set(null);
        this.editorVisible.set(true);
    }
    protected openEdit(funnel: Funnel) {
        this.editingFunnel.set(funnel);
        this.editorVisible.set(true);
    }
    protected readonly funnelActions = (funnel: Funnel): TableRowActionItem[] => {
        return [
            {
                label: this.transloco.translate('common.actions.edit'),
                icon: 'pi pi-pencil',
                command: () => this.openEdit(funnel)
            },
            {
                label: this.transloco.translate('common.actions.delete'),
                icon: 'pi pi-trash',
                danger: true,
                command: () => this.confirmDelete(funnel)
            }
        ];
    };
    private confirmDelete(funnel: Funnel) {
        this.confirmation.confirm({
            message: this.transloco.translate('funnels.manager.confirmDelete', {
                name: funnel.name
            }),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
            acceptButtonProps: dialogDangerButton(this.transloco.translate('common.actions.delete')),
            accept: () => this.deleteFunnel(funnel)
        });
    }
    private deleteFunnel(funnel: Funnel) {
        const site = this.siteService.activeSite();
        if (!site) return;
        this.deletingFunnelId.set(funnel.id);
        this.analytics
            .deleteFunnel(site.id, funnel.id)
            .pipe(finalize(() => this.deletingFunnelId.set(null)))
            .subscribe({
                next: () => {
                    if (site.id !== this.siteService.activeSite()?.id) return;
                    if (this.selectedFunnelId() === funnel.id) this.selectFunnel(null);
                    this.definitions.reload();
                }
            });
    }
    protected readonly onDefinitionsChanged = this.definitions.reload;
    protected refresh(mode: StatsQueryMode = 'blocking') {
        const site = this.siteService.activeSite();
        if (!site || this.loading()) return;
        if (this.loadError()) {
            if (mode === 'blocking') this.definitions.reload();
            return;
        }
        const dates = this.reportRange.currentDateRange();
        if (!dates) return;
        this.trafficRefreshKey.update((key) => key + 1);
        const selected = this.selectedFunnelId();
        this.loadReporting(site.id, dates.from, dates.to, selected ? [selected] : [], mode);
        if (selected) this.loadFunnelStats(site.id, selected, dates.from, dates.to);
    }
    private loadReporting(siteId: string, from: string, to: string, funnelIds: string[], mode: StatsQueryMode = 'blocking') {
        const sequence = ++this.reportingSequence;
        this.reportingRequest?.unsubscribe();
        const request = new Subscription();
        this.reportingRequest = request;
        this.statsQuery.load({ siteId, from, to, funnelIds, mode });
        const blocking = mode === 'blocking';
        if (blocking) this.seriesLoading.set(true);
        const comparison = previousPeriod(from, to);
        request.add(
            forkJoin({
                current: this.analytics.getFunnelTimeseries(siteId, from, to, funnelIds),
                previous: this.analytics.getFunnelTimeseries(siteId, comparison.from, comparison.to, funnelIds).pipe(catchError(() => of(null)))
            })
                .pipe(finalize(() => sequence === this.reportingSequence && this.seriesLoading.set(false)))
                .subscribe({
                    next: ({ current, previous }) => {
                        this.funnelSeries.set(current ?? []);
                        this.comparisonFunnelSeries.set(previous ?? []);
                        this.seriesRange.set({ from, to });
                        this.comparisonRange.set(previous ? comparison : null);
                    },
                    error: () => {
                        this.funnelSeries.set([]);
                        this.comparisonFunnelSeries.set([]);
                        this.seriesRange.set(null);
                        this.comparisonRange.set(null);
                    }
                })
        );
    }
    private loadFunnelStats(siteId: string, funnelId: string, from: string, to: string) {
        const sequence = ++this.funnelStatsSequence;
        this.funnelStatsRequest?.unsubscribe();
        this.funnelStatsLoading.set(true);
        this.funnelStatsRequest = this.analytics
            .getFunnelStats(siteId, funnelId, from, to)
            .pipe(finalize(() => sequence === this.funnelStatsSequence && this.funnelStatsLoading.set(false)))
            .subscribe({
                next: (stats) => sequence === this.funnelStatsSequence && this.funnelStats.set(stats),
                error: () => sequence === this.funnelStatsSequence && this.funnelStats.set(null)
            });
    }

    private clearFunnelStats() {
        this.funnelStatsSequence += 1;
        this.funnelStatsRequest?.unsubscribe();
        this.funnelStatsRequest = null;
        this.funnelStatsLoading.set(false);
        this.funnelStats.set(null);
    }
}
