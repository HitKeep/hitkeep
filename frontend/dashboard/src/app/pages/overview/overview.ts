import { ChangeDetectionStrategy, Component, DestroyRef, computed, effect, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { finalize, Subscription } from 'rxjs';
import { ButtonModule } from '@openng/optimus-ui/button';
import { CardModule } from '@openng/optimus-ui/card';
import { IconFieldModule } from '@openng/optimus-ui/iconfield';
import { InputIconModule } from '@openng/optimus-ui/inputicon';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { MessageModule } from '@openng/optimus-ui/message';
import { SelectModule } from '@openng/optimus-ui/select';
import { SkeletonModule } from '@openng/optimus-ui/skeleton';
import { injectActiveLang } from '@core/i18n/active-lang';
import { DateRange } from '@components/range-toolbar/range-toolbar';
import { ReportRangeToolbar } from '@components/report-range-toolbar/report-range-toolbar';
import { SiteFavicon } from '@features/sites/components/site-favicon';
import { SiteService } from '@features/sites/services/site.service';
import { StatsService } from '@features/analytics/services/stats.service';
import { ChartDataPoint, Site, SiteOverviewStats } from '@models/analytics.types';
import { MainLayoutContextService } from '@layout/main-layout-context.service';
import { PreferenceStorage } from '@services/preference-storage';
import { injectReportRange } from '@services/report-range-preferences.service';

type OverviewSortKey = 'domain' | 'pageviews' | 'visitors';
type OverviewSiteStatus = 'loading' | 'ready' | 'error';

interface SiteStatsState {
    status: OverviewSiteStatus;
    stats: SiteOverviewStats | null;
}

interface OverviewSortOption {
    label: string;
    value: OverviewSortKey;
}

type OverviewSparkline = { kind: 'dot'; y: number } | { kind: 'line'; points: string; areaPoints: string };

interface OverviewSiteRow {
    site: Site;
    status: OverviewSiteStatus;
    pageviews: number;
    pageviewsLabel: string;
    visitors: number;
    visitorsLabel: string;
    bounceRateLabel: string;
    sparkline: OverviewSparkline | null;
    sparklineAria: string;
    hasTraffic: boolean;
    searchText: string;
}

const SPARKLINE_WIDTH = 160;
const SPARKLINE_HEIGHT = 56;

const OVERVIEW_SORT_STORAGE_KEY = 'hitkeep.overviewSort';
const OVERVIEW_SORT_KEYS: readonly OverviewSortKey[] = ['domain', 'pageviews', 'visitors'];

@Component({
    selector: 'app-overview-page',
    imports: [FormsModule, ButtonModule, CardModule, IconFieldModule, InputIconModule, InputTextModule, MessageModule, ReportRangeToolbar, SelectModule, SiteFavicon, SkeletonModule, TranslocoPipe],
    templateUrl: './overview.html',
    styleUrl: './overview.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class OverviewPage {
    protected readonly siteService = inject(SiteService);
    private readonly transloco = inject(TranslocoService);
    private readonly localeService = inject(TranslocoLocaleService);
    private readonly statsService = inject(StatsService);
    private readonly destroyRef = inject(DestroyRef);
    protected readonly layoutContext = inject(MainLayoutContextService);
    private readonly preferenceStorage = inject(PreferenceStorage);
    private readonly reportRange = injectReportRange();
    private readonly activeLanguage = injectActiveLang();
    private statsRequest: Subscription | null = null;
    private loadSequence = 0;

    protected readonly searchTerm = signal('');
    protected readonly sortValue = signal<OverviewSortKey>(this.initialSort());
    protected readonly siteStates = signal<Record<string, SiteStatsState>>({});
    protected readonly isStatsLoading = signal(false);

    protected readonly sortOptions = computed<OverviewSortOption[]>(() => {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('overview.sort.domain'), value: 'domain' },
            { label: this.transloco.translate('overview.sort.pageviews'), value: 'pageviews' },
            { label: this.transloco.translate('overview.sort.visitors'), value: 'visitors' }
        ];
    });

    protected readonly siteRows = computed<OverviewSiteRow[]>(() => {
        this.activeLanguage();
        const term = this.searchTerm().trim().toLowerCase();
        const states = this.siteStates();
        const rows = this.siteService.sites().map((site) => this.buildSiteRow(site, states[site.id] ?? { status: 'loading', stats: null }));
        return rows.filter((row) => !term || row.searchText.includes(term)).sort((left, right) => this.compareRows(left, right));
    });

    constructor() {
        effect(() => {
            this.loadSiteStats(this.siteService.sites(), this.currentDateRange());
        });

        this.destroyRef.onDestroy(() => this.statsRequest?.unsubscribe());
    }

    protected onSearch(event: Event): void {
        this.searchTerm.set((event.target as HTMLInputElement | null)?.value ?? '');
    }

    protected setSort(value: OverviewSortKey | null): void {
        if (value === 'domain' || value === 'pageviews' || value === 'visitors') {
            this.sortValue.set(value);
            this.preferenceStorage.write(OVERVIEW_SORT_STORAGE_KEY, value);
        }
    }

    protected refreshOverview(): void {
        this.loadSiteStats(this.siteService.sites(), this.currentDateRange());
    }

    protected openAddSite(): void {
        this.layoutContext.openAddSite();
    }

    protected openDashboard(site: Site): void {
        this.layoutContext.onSiteSelected(site);
    }

    protected openDashboardFromKeyboard(event: KeyboardEvent, site: Site): void {
        if (event.key !== 'Enter' && event.key !== ' ') {
            return;
        }

        event.preventDefault();
        this.openDashboard(site);
    }

    private loadSiteStats(sites: Site[], range: DateRange | null): void {
        const sequence = ++this.loadSequence;
        this.statsRequest?.unsubscribe();

        if (!range || sites.length === 0) {
            this.siteStates.set({});
            this.isStatsLoading.set(false);
            return;
        }

        this.isStatsLoading.set(true);
        this.siteStates.set(
            Object.fromEntries(
                sites.map((site) => [
                    site.id,
                    {
                        status: 'loading',
                        stats: null
                    } satisfies SiteStatsState
                ])
            )
        );

        this.statsRequest = this.statsService
            .fetchSitesOverviewStats(range.from, range.to)
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => {
                    if (this.loadSequence === sequence) {
                        this.isStatsLoading.set(false);
                    }
                })
            )
            .subscribe({
                next: (response) => {
                    if (this.loadSequence !== sequence) {
                        return;
                    }

                    const statsBySite = new Map(response.sites.map((stats) => [stats.site_id, stats]));
                    this.siteStates.set(
                        Object.fromEntries(
                            sites.map((site) => {
                                const stats = statsBySite.get(site.id);
                                if (!stats || stats.status === 'error') {
                                    return [site.id, { status: 'error', stats: null } satisfies SiteStatsState];
                                }
                                return [site.id, { status: 'ready', stats } satisfies SiteStatsState];
                            })
                        )
                    );
                },
                error: () => {
                    if (this.loadSequence !== sequence) {
                        return;
                    }
                    this.siteStates.set(Object.fromEntries(sites.map((site) => [site.id, { status: 'error', stats: null } satisfies SiteStatsState])));
                }
            });
    }

    private buildSiteRow(site: Site, state: SiteStatsState): OverviewSiteRow {
        const stats = state.stats;
        const chartData = stats?.chart_data ?? [];
        const pageviews = stats?.total_pageviews ?? 0;
        const visitors = stats?.unique_sessions ?? 0;
        const hasTraffic = pageviews > 0 || visitors > 0 || chartData.some((point) => point.pageviews > 0 || point.visitors > 0);
        return {
            site,
            status: state.status,
            pageviews,
            pageviewsLabel: state.status === 'loading' ? '' : this.localeService.localizeNumber(pageviews, 'decimal'),
            visitors,
            visitorsLabel: state.status === 'loading' ? '' : this.localeService.localizeNumber(visitors, 'decimal'),
            bounceRateLabel: state.status === 'ready' && stats ? `${this.localeService.localizeNumber(stats.bounce_rate, 'decimal', undefined, { maximumFractionDigits: 1 })}%` : '-',
            sparkline: hasTraffic ? this.buildSparkline(chartData) : null,
            sparklineAria: this.transloco.translate('overview.chartAria', { site: site.domain }),
            hasTraffic,
            searchText: site.domain.toLowerCase()
        };
    }

    private compareRows(left: OverviewSiteRow, right: OverviewSiteRow): number {
        switch (this.sortValue()) {
            case 'pageviews':
                return right.pageviews - left.pageviews || left.site.domain.localeCompare(right.site.domain);
            case 'visitors':
                return right.visitors - left.visitors || left.site.domain.localeCompare(right.site.domain);
            default:
                return left.site.domain.localeCompare(right.site.domain);
        }
    }

    private buildSparkline(data: ChartDataPoint[]): OverviewSparkline | null {
        if (!data.length) return null;
        const values = data.map((point) => point.pageviews);
        const max = Math.max(...values, 1);
        const yForValue = (value: number) => SPARKLINE_HEIGHT - (value / max) * (SPARKLINE_HEIGHT - 4) - 2;
        if (values.length === 1) return { kind: 'dot', y: yForValue(values[0]) };
        const points = values
            .map((value, index) => {
                const x = (index / (values.length - 1)) * SPARKLINE_WIDTH;
                const y = yForValue(value);
                return `${x.toFixed(1)},${y.toFixed(1)}`;
            })
            .join(' ');
        return { kind: 'line', points, areaPoints: `0,${SPARKLINE_HEIGHT} ${points} ${SPARKLINE_WIDTH},${SPARKLINE_HEIGHT}` };
    }

    private initialSort(): OverviewSortKey {
        const stored = this.preferenceStorage.read<OverviewSortKey>(OVERVIEW_SORT_STORAGE_KEY);
        return stored !== null && OVERVIEW_SORT_KEYS.includes(stored) ? stored : 'domain';
    }

    private currentDateRange(): DateRange | null {
        return this.reportRange.currentDateRange();
    }
}
