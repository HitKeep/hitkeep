import { ChartLegend } from '@core/charts/chart-legend';
import { ChangeDetectionStrategy, Component, DOCUMENT, ElementRef, DestroyRef, afterRenderEffect, computed, input, inject, signal, untracked } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { NgxEchartsDirective } from 'ngx-echarts';
import type { ECharts, EChartsCoreOption, EChartsInitOpts } from 'echarts/core';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ChartDesignToggle } from '@components/chart-design-toggle/chart-design-toggle';
import { formatComparisonPeriod, type ComparisonPeriod } from '@core/analytics/comparison-period';
import { bindAnnotationInteractions, type AnnotationMarkerKind } from '@core/charts/annotation-interactions';
import { annotationMarkers } from '@core/charts/annotation-markers';
import { buildHitkeepChartMergeOptions, buildHitkeepChartOptions, hitkeepChartTheme, resolveChartColor, type HitkeepChartAnnotations, type HitkeepChartDesign, type HitkeepChartSeries } from '@core/charts/hitkeep-chart-options';
import { provideHitkeepEcharts } from '@core/charts/hitkeep-echarts.provider';
import { AnnotationStrip } from '@features/annotations/annotation-strip';
import { SiteAnnotationsService } from '@features/annotations/site-annotations.service';
import { ChartDesignPreferencesService } from '@services/chart-design-preferences.service';
import { PreferencesService } from '@services/preferences.service';
import { ThemeManagerService } from '@services/theme-manager.service';
import { injectSkeletonGate } from '@services/report-subject.service';
import { localeForLanguage } from '@core/i18n/duration-format';

export type SeriesChartPoint = Record<string, number | string | null> & { time: string };

export interface SeriesDefinition {
    key: string;
    label: string;
    color: string;
    gradientFrom?: string;
    gradientTo?: string;
    design?: HitkeepChartDesign;
    dashed?: boolean;
    smooth?: boolean;
    /** Set false for derived series (e.g. trend lines) that must not get a comparison twin. */
    includeInComparison?: boolean;
}

interface PointTooltipParam {
    dataIndex: number;
    seriesId: string;
}

interface InspectedPeriod {
    title: string;
    rows: { label: string; value: string }[];
}

@Component({
    selector: 'app-series-chart',
    changeDetection: ChangeDetectionStrategy.OnPush,
    imports: [ChartLegend, AnnotationStrip, ButtonModule, ChartDesignToggle, NgxEchartsDirective, TranslocoPipe],
    providers: [provideHitkeepEcharts()],
    template: `
        @let inspectable = hasData() && !showSkeleton();
        <div
            class="h-64 sm:h-72 min-w-0 w-full relative rounded focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-current"
            [attr.role]="inspectable ? 'application' : 'group'"
            [attr.tabindex]="inspectable ? 0 : null"
            [attr.aria-label]="accessibilityLabel()"
            [attr.aria-description]="inspectable ? ('comparison.inspectChartHint' | transloco) : null"
            [attr.aria-keyshortcuts]="inspectable ? 'ArrowLeft ArrowRight Escape' : null"
            (pointerdown)="pointerFocusing = true"
            (pointerup)="pointerFocusing = false"
            (pointercancel)="pointerFocusing = false"
            (focus)="!pointerFocusing && inspectionIndex.set(0)"
            (keydown)="inspectable && onInspectionKey($event)"
            (blur)="closeInspection()"
        >
            @if (showSkeleton()) {
                <div class="flex items-center justify-center h-full" aria-live="polite">
                    <i class="pi pi-spin pi-spinner text-4xl text-[var(--p-primary-color)]" aria-hidden="true"></i>
                </div>
            } @else if (hasData()) {
                <div echarts aria-hidden="true" class="h-full w-full" [options]="chartFrameOptions()" [initOpts]="chartInitOptions" [autoResize]="true" (chartInit)="onChartInit($event)"></div>
            } @else {
                <div class="absolute inset-0 flex flex-col items-center justify-center text-[var(--p-text-muted-color)] bg-[var(--p-surface-ground)]/50 rounded-lg border-2 border-dashed border-[var(--p-surface-border)] p-6 text-center">
                    <h3 class="font-semibold text-[var(--p-text-color)] text-lg mb-1">{{ emptyTitle() || ('common.empty.noDataTitle' | transloco) }}</h3>
                    <p class="text-sm max-w-xs">{{ emptyDescription() || ('common.empty.noDataDescription' | transloco) }}</p>
                    <div class="mt-4 empty:hidden">
                        <ng-content select="[chart-empty-actions]" />
                    </div>
                </div>
            }
        </div>
        @if (inspectable) {
            <span class="sr-only" role="status">{{ inspectionAnnouncement() }}</span>
            <app-chart-legend [series]="renderedSeries()" [instance]="chartInstance()" [hidden]="hiddenSeries()" [toolsAvailable]="notes.canWrite() || showDesignSelector()">
                <div class="mt-2 flex flex-wrap items-center justify-end gap-2">
                    @if (notes.canWrite()) {
                        <p-button
                            class="chart-note-button"
                            type="button"
                            size="small"
                            severity="secondary"
                            icon="pi pi-bookmark"
                            [text]="true"
                            [label]="'annotations.add' | transloco"
                            [ariaLabel]="'annotations.addAria' | transloco"
                            (onClick)="addNote()"
                        />
                    }
                    @if (showDesignSelector()) {
                        <app-chart-design-toggle [value]="effectiveDesign()" (valueChange)="setSelectedDesign($event)" />
                    }
                </div>
            </app-chart-legend>
            <app-annotation-strip [annotations]="markers().visible" [granularity]="granularity()" (selected)="notes.openExisting($event, granularity())" />
        }
    `
})
export class SeriesChart {
    data = input.required<SeriesChartPoint[]>();
    series = input.required<SeriesDefinition[]>();
    comparisonData = input<SeriesChartPoint[]>([]);
    isLoading = input<boolean>(false);
    minInterval = input(1);
    showZeroValues = input(false);
    valueFormatter = input<(value: number) => string>();
    isShortRange = input<boolean>(false);
    emptyTitle = input<string>('');
    emptyDescription = input<string>('');
    currentPeriod = input<ComparisonPeriod | null>(null);
    previousPeriod = input<ComparisonPeriod | null>(null);
    design = input<HitkeepChartDesign>('area');
    showDesignSelector = input<boolean>(true);
    ariaLabelKey = input<string>('common.seriesChartAria');

    protected readonly chartInitOptions: EChartsInitOpts = { renderer: 'canvas' };
    protected readonly chartInstance = signal<ECharts | null>(null);
    private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
    private readonly document = inject(DOCUMENT);
    protected pointerFocusing = false;
    protected readonly inspectionIndex = signal<number | null>(null);
    private readonly selection = signal<ReadonlyMap<string, boolean>>(new Map());
    private readonly hiddenSeries = computed(() => {
        const choice = this.selection();
        const current = this.hasCurrentData();
        return new Set(
            this.renderedSeries()
                .filter((item) => !(choice.get(item.id) ?? this.defaultVisible(item, current)))
                .map((item) => item.id)
        );
    });
    private prefs = inject(PreferencesService);
    private themeManager = inject(ThemeManagerService);
    private designPrefs = inject(ChartDesignPreferencesService);
    private localeService = inject(TranslocoLocaleService);
    private transloco = inject(TranslocoService);
    private activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });
    protected readonly notes = inject(SiteAnnotationsService);
    private unbindNotes: (() => void) | null = null;

    protected readonly effectiveDesign = computed(() => this.designPrefs.design() ?? this.design());
    protected readonly granularity = computed(() => (this.isShortRange() ? 'hour' : 'day'));
    protected readonly markers = computed(() => annotationMarkers(this.data() || [], this.notes.annotations()));

    private readonly chartAnnotations = computed((): HitkeepChartAnnotations => {
        this.activeLanguage();
        const { lines, areas } = this.markers();
        return {
            lines: lines.map(({ index, annotations }) => ({
                index,
                label: annotations.length > 1 ? this.transloco.translate('annotations.chart.grouped', { count: annotations.length, body: annotations[0].body }) : annotations[0].body
            })),
            areas: areas.map(({ start, end, annotation }) => ({ start, end, label: annotation.body }))
        };
    });

    protected readonly pairedComparison = computed(() => {
        const current = this.data();
        const previous = this.comparisonData();
        if (!previous.length || current.length !== previous.length) return [];

        const currentTimes = current.map((point) => Date.parse(point.time));
        const previousTimes = previous.map((point) => Date.parse(point.time));
        const offset = currentTimes[0] - previousTimes[0];
        if (!Number.isFinite(offset)) return [];

        const hasSameIntervals = currentTimes.every((time, index) => time - previousTimes[index] === offset && (index === 0 || time > currentTimes[index - 1]));
        return hasSameIntervals || (isMonthlyGrid(currentTimes) && isMonthlyGrid(previousTimes)) ? previous : [];
    });

    protected readonly hasComparableSeries = computed(() => this.series().some((series) => series.includeInComparison !== false));
    protected readonly hasPairedComparison = computed(() => this.pairedComparison().length > 0 && this.hasComparableSeries());

    private readonly hasCurrentData = computed(() => this.containsData(this.data(), this.series()));
    protected readonly hasData = computed(
        () =>
            this.hasCurrentData() ||
            this.containsData(
                this.pairedComparison(),
                this.series().filter((item) => item.includeInComparison !== false)
            )
    );

    private containsData(points: SeriesChartPoint[], series: SeriesDefinition[]): boolean {
        const includeZero = this.showZeroValues();
        return points.some((point) =>
            series.some(({ key }) => {
                const value = pointValue(point[key]);
                return value !== null && (includeZero || value !== 0);
            })
        );
    }

    protected readonly showSkeleton = injectSkeletonGate(this.isLoading, this.hasData);

    protected accessibilityLabel = computed(() => {
        const locale = localeForLanguage(this.activeLanguage());
        const count = this.data()?.length || 0;
        const descriptions = [this.transloco.translate(this.ariaLabelKey(), { count })];
        const current = this.currentPeriod();
        const previous = this.previousPeriod();
        if (current) descriptions.push(`${this.transloco.translate('comparison.currentPeriod')}, ${formatComparisonPeriod(current, locale)}.`);
        if (previous && this.hasPairedComparison() && this.hasData()) {
            descriptions.push(`${this.transloco.translate('comparison.previousPeriod')}, ${formatComparisonPeriod(previous, locale)}.`);
        } else if (this.hasComparableSeries() && this.comparisonData().length && !this.pairedComparison().length) {
            descriptions.push(this.transloco.translate('comparison.gridMismatch'));
        }
        const notes = this.markers().visible.length;
        if (notes > 0) descriptions.push(this.transloco.translate('annotations.chart.aria', { count: notes }));
        return descriptions.join(' ');
    });

    protected chartFrameOptions = computed((): EChartsCoreOption => {
        this.activeLanguage();
        this.themeManager.version();
        const series = this.chartSeries([], [], this.hasPairedComparison());
        const frame = buildHitkeepChartOptions({
            ariaLabel: this.transloco.translate(this.ariaLabelKey(), { count: 0 }),
            design: this.effectiveDesign(),
            labels: [],
            locale: this.transloco.getActiveLang(),
            series,
            valueFormatter: this.valueFormatter(),
            minInterval: this.minInterval(),
            theme: hitkeepChartTheme(this.prefs.isDarkMode())
        });
        const choices = untracked(this.selection);
        const current = this.hasCurrentData();
        return {
            ...frame,
            legend: { ...frame.legend, selected: Object.fromEntries(series.map((item) => [item.label, choices.get(item.id) ?? this.defaultVisible(item, current)])) },
            tooltip: { ...frame.tooltip, formatter: this.formatPointTooltip, triggerOn: 'mousemove|click' }
        };
    });

    protected chartMergeOptions = computed((): EChartsCoreOption => {
        this.activeLanguage();
        this.themeManager.version();
        const raw = this.data() || [];
        const bucketLabel = this.bucketLabelFormatter(raw);
        const start = this.currentPeriod()?.from;
        return buildHitkeepChartMergeOptions({
            ariaLabel: this.accessibilityLabel(),
            design: this.effectiveDesign(),
            labels: raw.map((point, index) => (index === 0 && start && Date.parse(point.time) < Date.parse(start) ? this.pointFormatters().minute.format(new Date(start)) : bucketLabel.format(new Date(point.time)))),
            locale: this.transloco.getActiveLang(),
            series: this.renderedSeries(),
            theme: hitkeepChartTheme(this.prefs.isDarkMode())
        });
    });

    constructor() {
        this.notes.load();
        inject(DestroyRef).onDestroy(() => this.unbindNotes?.());

        // ngx-echarts merges without `replaceMerge`, which only stayed invisible
        // while every reload tore the chart down. Now that a range switch keeps
        // the instance alive, a series the new data dropped — a comparison twin,
        // a conditional metric — would otherwise linger on screen forever.
        //
        // This runs after render on purpose: a theme, language or design change
        // makes the directive re-apply the frame with `notMerge`, and the data
        // patch has to land after that reset rather than before it.
        afterRenderEffect(() => {
            // Bail before reading the options: with no live chart there is
            // nothing to patch, and building them would relabel every bucket
            // for a spinner or an empty state. A new instance re-runs this.
            const chart = this.chartInstance();
            if (!chart || chart.isDisposed()) return;
            chart.setOption(this.chartMergeOptions(), { replaceMerge: ['series'] });
        });
        afterRenderEffect(() => {
            const chart = this.chartInstance();
            const index = this.activeInspectionIndex();
            if (!chart || chart.isDisposed() || index === null) return;
            const first = this.renderedSeries().findIndex(({ id, data }) => data[index] !== null && !this.hiddenSeries().has(id));
            if (first >= 0) chart.dispatchAction({ type: 'showTip', seriesIndex: first, dataIndex: index });
            else chart.dispatchAction({ type: 'hideTip' });
        });
    }

    protected onChartInit(chart: ECharts): void {
        this.chartInstance.set(chart);
        this.unbindNotes?.();
        const unbind = bindAnnotationInteractions(chart, {
            canWrite: () => this.notes.canWrite(),
            bucketCount: () => this.data()?.length ?? 0,
            isBar: () => this.effectiveDesign() === 'bar',
            onAdd: (start, end) => this.openNewNote(start, end),
            onOpen: (kind, index) => this.openMarker(kind, index)
        });
        const legendChange = (event: unknown) => {
            const { name, selected } = event as { name: string; selected: Record<string, boolean> };
            const item = this.renderedSeries().find((series) => series.label === name);
            if (item) this.selection.update((choices) => new Map(choices).set(item.id, selected[name]));
        };
        chart.on('legendselectchanged', legendChange);
        this.unbindNotes = () => {
            unbind();
            chart.off('legendselectchanged', legendChange);
        };
    }

    private readonly activeInspectionIndex = computed(() => {
        const index = this.inspectionIndex();
        const length = this.data().length;
        return index === null || !length ? null : Math.min(index, length - 1);
    });

    protected readonly renderedSeries = computed(() => {
        this.activeLanguage();
        this.themeManager.version();
        return this.chartSeries(this.data(), this.pairedComparison());
    });

    protected readonly inspectionAnnouncement = computed(() => {
        const index = this.activeInspectionIndex();
        if (index === null) return '';
        const visible = this.readableSeries(this.renderedSeries().map(({ id }) => id));
        const detail = this.pointDetails(index, visible)
            .flatMap(({ title, rows }) => [title, ...rows.map(({ label, value }) => `${label}: ${value}`)])
            .join('. ');
        return detail || `${this.formatPointTime(this.data()[index].time)}. ${this.transloco.translate('comparison.pointUnavailable')}`;
    });

    private readonly pointFormatters = computed(() => {
        const locale = localeForLanguage(this.activeLanguage());
        const options = { year: 'numeric', month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit', timeZoneName: 'shortOffset' } as const;
        return {
            day: new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeZone: 'UTC' }),
            minute: new Intl.DateTimeFormat(locale, options),
            second: new Intl.DateTimeFormat(locale, { ...options, second: '2-digit' }),
            millisecond: new Intl.DateTimeFormat(locale, { ...options, second: '2-digit', fractionalSecondDigits: 3 }),
            value: new Intl.NumberFormat(locale, { maximumFractionDigits: 2 })
        };
    });

    protected onInspectionKey(event: KeyboardEvent): void {
        if (event.key === 'Escape') {
            this.closeInspection();
            return;
        }
        const step = event.key === 'ArrowLeft' ? -1 : event.key === 'ArrowRight' ? 1 : 0;
        if (!step) return;
        event.preventDefault();
        const index = this.activeInspectionIndex() ?? (step > 0 ? -1 : this.data().length);
        this.inspectionIndex.set(Math.max(0, Math.min(index + step, this.data().length - 1)));
    }

    protected closeInspection(): void {
        if (this.inspectionIndex() === null) return;
        this.inspectionIndex.set(null);
        const chart = this.chartInstance();
        if (chart && !chart.isDisposed()) chart.dispatchAction({ type: 'hideTip' });
    }

    private defaultVisible(item: HitkeepChartSeries, hasCurrent: boolean): boolean {
        return item.muted ? !hasCurrent : !item.dashed;
    }

    // A matched prior value remains available on inspection before its overlay is drawn.
    private readableSeries(ids: string[]): Set<string> {
        const choices = this.selection();
        const visible = new Set(ids.filter((id) => !this.hiddenSeries().has(id)));
        for (const { key } of this.series()) {
            const prior = key + '-comparison';
            if (choices.get(prior) !== false && (visible.has(key) || (ids.includes(prior) && choices.get(key) !== false))) visible.add(prior);
        }
        return visible;
    }

    private pointDetails(index: number, seriesIds: ReadonlySet<string>): InspectedPeriod[] {
        const format = this.pointFormatters();
        const periods: InspectedPeriod[] = [];
        for (const previous of [false, true]) {
            const point = (previous ? this.pairedComparison() : this.data())[index];
            if (!point) continue;
            const rows = this.series().flatMap((series) => {
                const id = previous ? `${series.key}-comparison` : series.key;
                const value = pointValue(point[series.key]);
                if (!seriesIds.has(id) || (previous && series.includeInComparison === false) || value === null) return [];
                return [{ label: series.label, value: (this.valueFormatter() ?? format.value.format)(value) }];
            });
            if (!rows.length) continue;
            const period = previous ? this.previousPeriod() : this.currentPeriod();
            const time = this.formatPointTime(point.time);
            let title = `${this.transloco.translate(previous ? 'comparison.previousPeriod' : 'comparison.currentPeriod')} · ${this.transloco.translate('comparison.bucketStart', { time })}`;
            if (period && Date.parse(point.time) < Date.parse(period.from)) {
                title += ` · ${this.transloco.translate('comparison.countedFrom', { time: this.formatPointTime(period.from) })}`;
            }
            periods.push({ title, rows });
        }
        return periods;
    }

    private formatPointTime(time: string): string {
        const date = new Date(time);
        const format = this.pointFormatters();
        const formatter = time.length === 10 ? format.day : date.getMilliseconds() ? format.millisecond : date.getSeconds() ? format.second : format.minute;
        return formatter.format(date);
    }

    private readonly formatPointTooltip = (params: PointTooltipParam | PointTooltipParam[]): HTMLElement => {
        const items = Array.isArray(params) ? params : [params];
        const root = this.document.createElement('div');
        root.className = 'hk-chart-point-tooltip';
        root.setAttribute('aria-hidden', 'true');
        root.style.whiteSpace = 'normal';
        root.style.maxWidth = 'min(18rem, 65vw)';
        const index = items[0]?.dataIndex;
        if (index === undefined) return root;
        const ids = this.readableSeries(items.map(({ seriesId }) => seriesId));
        for (const { title, rows } of this.pointDetails(index, ids)) {
            const section = this.document.createElement('div');
            if (root.childElementCount) section.style.marginTop = '0.5rem';
            const heading = this.document.createElement('strong');
            heading.textContent = title;
            section.append(heading);
            for (const { label, value } of rows) {
                const row = this.document.createElement('div');
                row.textContent = `${label}: ${value}`;
                section.append(row);
            }
            root.append(section);
        }
        return root;
    };

    /** The toolbar button: a point on the newest bucket, which the reader can move in the dialog. */
    protected addNote(): void {
        const last = (this.data()?.length ?? 0) - 1;
        if (last >= 0) {
            this.openNewNote(last, null);
        }
    }

    private noteTrigger(): HTMLElement | null {
        const button = this.host.nativeElement.querySelector<HTMLElement>('.chart-note-button button');
        return button?.checkVisibility() ? button : this.host.nativeElement.querySelector<HTMLElement>('[aria-keyshortcuts]');
    }

    private openNewNote(start: number, end: number | null): void {
        const data = this.data() ?? [];
        const startPoint = data[start];
        if (!startPoint) {
            return;
        }
        const endPoint = end === null ? undefined : data[end];
        this.notes.open({ startsAt: new Date(startPoint.time), endsAt: endPoint ? new Date(endPoint.time) : null, body: '', granularity: this.granularity() }, this.noteTrigger());
    }

    private openMarker(kind: AnnotationMarkerKind, index: number): void {
        const markers = this.markers();
        const annotation = kind === 'line' ? markers.lines[index]?.annotations[0] : markers.areas[index]?.annotation;
        if (annotation) {
            this.notes.openExisting(annotation, this.granularity(), this.noteTrigger());
        }
    }

    protected setSelectedDesign(value: HitkeepChartDesign): void {
        this.designPrefs.setDesign(value);
    }

    private chartSeries(raw: SeriesChartPoint[], cmp: SeriesChartPoint[], includeComparison = cmp.length > 0): HitkeepChartSeries[] {
        const series = this.series() || [];
        const chartSeries: HitkeepChartSeries[] = series.map((s) => ({
            id: s.key,
            label: s.label,
            data: raw.map((d) => pointValue(d[s.key])),
            color: resolveChartColor(s.color),
            gradientFrom: s.gradientFrom,
            gradientTo: s.gradientTo,
            design: s.design,
            dashed: s.dashed,
            smooth: s.smooth
        }));
        // Notes explain the current period, so they ride on its first series only.
        if (raw.length > 0 && chartSeries[0]) {
            chartSeries[0].annotations = this.chartAnnotations();
        }

        if (includeComparison) {
            for (const s of series) {
                if (s.includeInComparison === false) {
                    continue;
                }
                chartSeries.push({
                    id: `${s.key}-comparison`,
                    label: this.transloco.translate('comparison.seriesLabel', { metric: s.label }),
                    data: cmp.map((d) => pointValue(d[s.key])),
                    color: resolveChartColor(s.color),
                    muted: true,
                    dashed: true
                });
            }
        }

        return chartSeries;
    }

    /**
     * One formatter for the whole axis. A year of daily buckets would otherwise
     * construct a few hundred `Intl.DateTimeFormat`s per rebuild, which is the
     * most expensive part of the range switch this chart animates through.
     */
    private bucketLabelFormatter(points: SeriesChartPoint[]): Intl.DateTimeFormat {
        const options: Intl.DateTimeFormatOptions = this.isShortRange() ? { hour: 'numeric', minute: '2-digit' } : { month: 'short', day: 'numeric' };
        if (points[0]?.time.length === 10) options.timeZone = 'UTC';
        return new Intl.DateTimeFormat(this.localeService.getLocale(), options);
    }
}

function pointValue(value: number | string | null | undefined): number | null {
    if (value == null || (typeof value === 'string' && !value.trim())) return null;
    const numeric = Number(value);
    return Number.isFinite(numeric) ? numeric : null;
}

// UTC month buckets are consecutive calendar months and can differ in elapsed days.
function isMonthlyGrid(times: number[]): boolean {
    const start = new Date(times[0]);
    if (start.getUTCDate() !== 1 || times[0] % 86_400_000 !== 0) return false;
    return times.every((time, index) => {
        const month = new Date(start);
        month.setUTCMonth(start.getUTCMonth() + index);
        return month.getTime() === time;
    });
}
