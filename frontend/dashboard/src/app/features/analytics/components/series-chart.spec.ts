import { ComponentFixture, TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { TranslocoService, TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { SeriesChart } from '@features/analytics/components/series-chart';
import { ReportSubjectService } from '@services/report-subject.service';
import { SiteAnnotationsService } from '@features/annotations/site-annotations.service';
import type { Annotation } from '@models/analytics.types';
import { signal } from '@angular/core';
import { vi } from 'vitest';

function stubAnnotations() {
    return {
        annotations: signal<Annotation[]>([]),
        canWrite: signal(false),
        load: vi.fn(),
        open: vi.fn(),
        openExisting: vi.fn()
    };
}

/** A live ECharts instance as far as SeriesChart touches it. */
function liveChart(setOption = vi.fn(), disposed = false) {
    const handlers = new Map<string, (params: unknown) => void>();
    return {
        setOption,
        dispatchAction: vi.fn(),
        handlers,
        isDisposed: () => disposed,
        on: (name: string, fn: (params: unknown) => void) => handlers.set(name, fn),
        off: vi.fn(),
        getDom: () => document.createElement('div'),
        getZr: () => ({ on: vi.fn(), off: vi.fn(), setCursorStyle: vi.fn() })
    };
}

describe('SeriesChart', () => {
    let component: SeriesChart;
    let fixture: ComponentFixture<SeriesChart>;
    let notes: ReturnType<typeof stubAnnotations>;

    const tooltipFormatter = () => (component as unknown as { chartFrameOptions: () => { tooltip: { formatter: (params: { dataIndex: number; seriesId: string }[]) => HTMLElement } } }).chartFrameOptions().tooltip.formatter;

    beforeEach(async () => {
        notes = stubAnnotations();
        await TestBed.configureTestingModule({
            imports: [
                SeriesChart,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            comparison: {
                                seriesLabel: '{{metric}} (previous)',
                                currentPeriod: 'Current period',
                                previousPeriod: 'Previous period',
                                gridMismatch: 'Different bucket grids',
                                bucketStart: 'Bucket starting {{time}}',
                                inspectChart: 'Inspect chart'
                            }
                        },
                        de: {
                            comparison: {
                                seriesLabel: '{{metric}} (vorher)',
                                currentPeriod: 'Aktueller Zeitraum',
                                previousPeriod: 'Vorheriger Zeitraum',
                                bucketStart: 'Intervallbeginn {{time}}',
                                inspectChart: 'Diagramm prüfen'
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en', 'de'],
                        reRenderOnLangChange: true,
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [
                { provide: SiteAnnotationsService, useValue: notes },
                provideTranslocoLocale({
                    defaultLocale: 'en-US',
                    langToLocaleMapping: {
                        en: 'en-US',
                        'en-US': 'en-US'
                    }
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(SeriesChart);
        component = fixture.componentInstance;
        fixture.componentRef.setInput('data', []);
        fixture.componentRef.setInput('series', []);
        fixture.detectChanges();
    });

    it('keeps the empty state accessible as a group', () => {
        const container = fixture.debugElement.query(By.css('div[role="group"]'));
        expect(container).toBeTruthy();
        expect(container.nativeElement.getAttribute('aria-label')).toBeTruthy();
    });

    it('renders the ECharts directive instead of the OptimusUI chart element when data exists', () => {
        fixture.componentRef.setInput('data', [
            { time: '2026-07-01T00:00:00Z', count: 5 },
            { time: '2026-07-02T00:00:00Z', count: 9 }
        ]);
        fixture.componentRef.setInput('series', [
            {
                key: 'count',
                label: 'Events',
                color: '#2563eb',
                gradientFrom: 'rgba(37, 99, 235, 0.3)',
                gradientTo: 'rgba(37, 99, 235, 0)'
            }
        ]);
        fixture.detectChanges();

        expect(fixture.debugElement.query(By.css('[echarts]'))).toBeTruthy();
        expect(fixture.debugElement.query(By.css('app-chart-design-toggle'))).toBeTruthy();
        expect(fixture.debugElement.query(By.css('p-' + 'chart'))).toBeNull();
    });

    it('inspects a date-only series without a comparison and keeps sampled zero distinct from a gap', () => {
        fixture.componentRef.setInput('data', [
            { time: '2026-11-12', value: null },
            { time: '2026-11-13', value: 0 }
        ]);
        fixture.componentRef.setInput('series', [{ key: 'value', label: 'INP', color: '#2563eb' }]);
        fixture.componentRef.setInput('valueFormatter', (value: number) => `${value} ms`);
        fixture.componentRef.setInput('showZeroValues', true);
        fixture.detectChanges();
        expect(fixture.nativeElement.querySelector('button[aria-keyshortcuts]')).toBeTruthy();
        const read = (dataIndex: number) => tooltipFormatter()([{ dataIndex, seriesId: 'value' }]).textContent;
        expect(read(0)).toBe('');
        expect(read(1)).toContain('Nov 13, 2026');
        expect(read(1)).toContain('INP: 0 ms');
    });

    it('applies comparison series, chart design variants, and merge updates', () => {
        fixture.componentRef.setInput('data', [
            { time: '2026-07-01T00:00:00Z', count: 5 },
            { time: '2026-07-02T00:00:00Z', count: 9 }
        ]);
        fixture.componentRef.setInput('comparisonData', [
            { time: '2026-06-29T00:00:00Z', count: 3 },
            { time: '2026-06-30T00:00:00Z', count: 4 }
        ]);
        fixture.componentRef.setInput('series', [
            {
                key: 'count',
                label: 'Events',
                color: '#2563eb',
                gradientFrom: 'rgba(37, 99, 235, 0.3)',
                gradientTo: 'rgba(37, 99, 235, 0)'
            }
        ]);
        fixture.componentRef.setInput('design', 'bar');
        fixture.detectChanges();

        const inspectable = component as unknown as {
            chartFrameOptions: () => { series: { data: number[] }[] };
            chartMergeOptions: () => { xAxis: { data: string[] }; series: { name: string; type: string; data: number[]; lineStyle?: { type?: string } }[] };
        };
        const frame = inspectable.chartFrameOptions();
        const merge = inspectable.chartMergeOptions();
        expect(merge.series.find((series) => series.name === 'Events')?.type).toBe('bar');
        expect(merge.series.find((series) => series.name === 'Events (previous)')?.lineStyle?.type).toBe('dashed');
        expect(frame.series.find((series) => series.data.length > 0)).toBeUndefined();
        expect(merge.xAxis.data.length).toBe(2);
        expect(merge.series.find((series) => series.name === 'Events')?.data).toEqual([5, 9]);
    });

    it('groups actual offset timestamps across midnight and includes genuine zero values', () => {
        const local = (day: number, hour: number, minute: number) => new Date(2026, 6, day, hour, minute).toISOString();
        fixture.componentRef.setInput('data', [
            { time: local(1, 23, 45), count: 0 },
            { time: local(2, 0, 15), sessions: 1 }
        ]);
        fixture.componentRef.setInput('comparisonData', [
            { time: local(1, 22, 45), count: 5, sessions: Number.POSITIVE_INFINITY },
            { time: local(1, 23, 15), count: 6 }
        ]);
        fixture.componentRef.setInput('series', [
            { key: 'count', label: '<b>Events</b>', color: '#2563eb' },
            { key: 'sessions', label: 'Sessions', color: '#10b981' }
        ]);
        fixture.componentRef.setInput('currentPeriod', { from: local(1, 23, 35), to: local(2, 0, 25) });
        fixture.componentRef.setInput('previousPeriod', { from: local(1, 22, 35), to: local(1, 23, 25) });
        fixture.detectChanges();

        const options = (component as unknown as { chartMergeOptions: () => { series: { name: string; data: (number | null)[]; connectNulls?: boolean }[] } }).chartMergeOptions;
        expect(fixture.debugElement.query(By.css('[echarts]'))).toBeTruthy();
        expect(fixture.nativeElement.querySelector('app-comparison-periods')).toBeNull();
        expect(fixture.nativeElement.querySelector('[role="img"]').getAttribute('aria-label')).toContain('Current period');
        expect(options().series[0]).toMatchObject({ data: [0, null], connectNulls: false });
        expect(options().series[2]).toMatchObject({ name: '<b>Events</b> (previous)', data: [5, 6] });
        expect(options().series[3]).toMatchObject({ data: [null, null], connectNulls: false });

        const formatter = tooltipFormatter();
        const pointsAt = (dataIndex: number) => ['count', 'count-comparison'].map((seriesId) => ({ dataIndex, seriesId }));
        const both = formatter(pointsAt(0));
        expect(Array.from(both.querySelectorAll('strong'), (heading) => heading.textContent)).toEqual([expect.stringContaining('11:45'), expect.stringContaining('10:45')]);
        expect(both.textContent).toContain('<b>Events</b>: 0');
        expect(both.querySelector('b')).toBeNull();
        const priorOnly = formatter(pointsAt(1));
        expect(priorOnly.querySelectorAll('strong')).toHaveLength(1);
        expect(priorOnly.textContent).toContain('11:15');
        expect(priorOnly.textContent).toContain('<b>Events</b>: 6');
        const acrossMidnight = formatter([...pointsAt(1), { dataIndex: 1, seriesId: 'sessions' }]);
        expect(Array.from(acrossMidnight.querySelectorAll('strong'), (heading) => heading.textContent)).toEqual([expect.stringContaining('12:15'), expect.stringContaining('11:15')]);
        expect(acrossMidnight.textContent).toContain('Sessions: 1');
        expect(acrossMidnight.textContent).not.toContain('Sessions: 0');

        TestBed.inject(TranslocoService).setActiveLang('de');
        fixture.detectChanges();
        expect(options().series[2].name).toBe('<b>Events</b> (vorher)');
        expect(formatter(pointsAt(0)).textContent).toContain('Vorheriger Zeitraum');
        expect(formatter(pointsAt(1)).textContent).toContain('23:15');
    });

    it('inspects points by keyboard, announces visible series, and dismisses with Escape', async () => {
        fixture.componentRef.setInput('data', [
            { time: '2026-07-01T00:00:00Z', count: 5 },
            { time: '2026-07-02T00:00:00Z', count: 7 }
        ]);
        fixture.componentRef.setInput('comparisonData', [
            { time: '2026-06-29T00:00:00Z', count: 3 },
            { time: '2026-06-30T00:00:00Z', count: 9 }
        ]);
        fixture.componentRef.setInput('series', [{ key: 'count', label: 'Events', color: '#2563eb' }]);
        fixture.detectChanges();
        await fixture.whenStable();
        const chart = liveChart();
        (component as unknown as { onChartInit: (chart: unknown) => void }).onChartInit(chart);
        const button = fixture.nativeElement.querySelector('button[aria-keyshortcuts]') as HTMLButtonElement;
        const status = fixture.nativeElement.querySelector('[role="status"]') as HTMLElement;
        const press = (key: string) => {
            button.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
            fixture.detectChanges();
        };
        button.click();
        await fixture.whenStable();
        expect(status.textContent).toContain('Events: 3');
        press('ArrowRight');
        await fixture.whenStable();
        expect(chart.dispatchAction).toHaveBeenLastCalledWith({ type: 'showTip', seriesIndex: 0, dataIndex: 1 });
        expect(status.textContent).toContain('Events: 9');
        chart.handlers.get('legendselectchanged')!({ selected: { 'Events (previous)': false } });
        await fixture.whenStable();
        expect(status.textContent).not.toContain('Previous period');
        expect(tooltipFormatter()([{ dataIndex: 1, seriesId: 'count-comparison' }]).textContent).toBe('');
        press('Escape');
        await vi.waitFor(() => {
            expect(button.getAttribute('aria-pressed')).toBe('false');
            expect(status.textContent).toBe('');
        });
    });

    it.each([
        { previous: [{ time: '2026-06-28T00:00:00Z', count: 5 }] },
        {
            previous: [
                { time: '2026-06-28T00:00:00Z', count: 5 },
                { time: '2026-06-28T01:00:00Z', count: 8 }
            ]
        }
    ])('omits an incompatible previous grid (%j)', ({ previous }) => {
        fixture.componentRef.setInput('data', [
            { time: '2026-07-01T00:00:00Z', count: 7 },
            { time: '2026-07-02T00:00:00Z', count: 9 }
        ]);
        fixture.componentRef.setInput('comparisonData', previous);
        fixture.componentRef.setInput('series', [{ key: 'count', label: 'Events', color: '#2563eb' }]);
        fixture.detectChanges();

        const merge = (component as unknown as { chartMergeOptions: () => { series: { name: string }[] } }).chartMergeOptions();
        expect(merge.series.map((series) => series.name)).toEqual(['Events']);
        expect(fixture.nativeElement.querySelector('[role="img"]').getAttribute('aria-label')).toContain('Different bucket grids');
        const tip = tooltipFormatter()([
            { dataIndex: 0, seriesId: 'count' },
            { dataIndex: 0, seriesId: 'count-comparison' }
        ]);
        expect(tip.textContent).toContain('Events: 7');
        expect(tip.textContent).not.toContain('Previous period');
    });

    it('only describes a previous series when one is plotted', () => {
        fixture.componentRef.setInput('data', [{ time: '2026-07-01T00:00:00Z', count: 0, trend: 3 }]);
        fixture.componentRef.setInput('comparisonData', [{ time: '2026-06-30T00:00:00Z', count: 0, trend: 2 }]);
        const trend = { key: 'trend', label: 'Trend', color: '#2563eb', includeInComparison: false };
        fixture.componentRef.setInput('series', [trend]);
        fixture.detectChanges();

        const merge = (component as unknown as { chartMergeOptions: () => { series: { name: string }[] } }).chartMergeOptions;
        expect(merge().series.map((series) => series.name)).toEqual(['Trend']);
        expect(fixture.nativeElement.querySelector('[role="img"]').getAttribute('aria-label')).not.toContain('Previous period');

        fixture.componentRef.setInput('series', [{ key: 'count', label: 'Events', color: '#2563eb' }]);
        fixture.detectChanges();
        expect(fixture.debugElement.query(By.css('[echarts]'))).toBeNull();
        expect(fixture.nativeElement.querySelector('[role="group"]').getAttribute('aria-label')).not.toContain('Previous period');
    });

    it('patches a live chart with replaceMerge so dropped series cannot linger', async () => {
        const setOption = vi.fn();
        const chart = liveChart(setOption);
        fixture.componentRef.setInput('data', [{ time: '2026-07-01T00:00:00Z', count: 5 }]);
        fixture.componentRef.setInput('comparisonData', [{ time: '2026-06-30T00:00:00Z', count: 3 }]);
        fixture.componentRef.setInput('series', [{ key: 'count', label: 'Events', color: '#2563eb' }]);
        fixture.detectChanges();

        (component as unknown as { onChartInit: (chart: unknown) => void }).onChartInit(chart);
        fixture.detectChanges();
        await fixture.whenStable();

        const firstPatch = setOption.mock.lastCall as [{ series: { name: string }[] }, { replaceMerge: string[] }];
        expect(firstPatch[1]).toEqual({ replaceMerge: ['series'] });
        expect(firstPatch[0].series.map((series) => series.name)).toContain('Events (previous)');

        fixture.componentRef.setInput('comparisonData', []);
        fixture.detectChanges();
        await fixture.whenStable();

        const secondPatch = setOption.mock.lastCall as [{ series: { name: string }[] }, { replaceMerge: string[] }];
        expect(secondPatch[0].series.map((series) => series.name)).toEqual(['Events']);
    });

    it('leaves a disposed chart alone', async () => {
        const setOption = vi.fn();
        fixture.componentRef.setInput('data', [{ time: '2026-07-01T00:00:00Z', count: 5 }]);
        fixture.componentRef.setInput('series', [{ key: 'count', label: 'Events', color: '#2563eb' }]);
        fixture.detectChanges();

        (component as unknown as { onChartInit: (chart: unknown) => void }).onChartInit(liveChart(setOption, true));
        fixture.componentRef.setInput('data', [{ time: '2026-07-02T00:00:00Z', count: 9 }]);
        fixture.detectChanges();
        await fixture.whenStable();

        expect(setOption).not.toHaveBeenCalled();
    });

    it('keeps the rendered chart through a reload of the same subject', async () => {
        fixture.componentRef.setInput('data', [{ time: '2026-07-01T00:00:00Z', count: 5 }]);
        fixture.componentRef.setInput('series', [{ key: 'count', label: 'Events', color: '#2563eb' }]);
        fixture.detectChanges();
        expect(fixture.debugElement.query(By.css('[echarts]'))).toBeTruthy();

        fixture.componentRef.setInput('isLoading', true);
        await fixture.whenStable();

        expect(fixture.debugElement.query(By.css('[echarts]'))).toBeTruthy();
        expect(fixture.debugElement.query(By.css('.pi-spinner'))).toBeNull();
    });

    it('shows the spinner for a first load and after the subject changes', async () => {
        fixture.componentRef.setInput('data', [{ time: '2026-07-01T00:00:00Z', count: 5 }]);
        fixture.componentRef.setInput('series', [{ key: 'count', label: 'Events', color: '#2563eb' }]);
        fixture.detectChanges();

        TestBed.inject(ReportSubjectService).set('another-site');
        fixture.componentRef.setInput('isLoading', true);
        await fixture.whenStable();

        expect(fixture.debugElement.query(By.css('.pi-spinner'))).toBeTruthy();
        expect(fixture.debugElement.query(By.css('[echarts]'))).toBeNull();
    });

    describe('annotations', () => {
        const days = [
            { time: '2026-07-01T00:00:00Z', count: 5 },
            { time: '2026-07-02T00:00:00Z', count: 9 },
            { time: '2026-07-03T00:00:00Z', count: 4 }
        ];
        const launch: Annotation = { id: 'n1', site_id: 's', starts_at: '2026-07-02T09:00:00Z', body: 'Launch', created_at: '2026-07-02T09:00:00Z' };
        const campaign: Annotation = { id: 'n2', site_id: 's', starts_at: '2026-07-01T00:00:00Z', ends_at: '2026-07-03T00:00:00Z', body: 'Campaign', created_at: '2026-07-01T00:00:00Z' };

        beforeEach(() => {
            fixture.componentRef.setInput('data', days);
            fixture.componentRef.setInput('series', [{ key: 'count', label: 'Events', color: '#2563eb' }]);
        });

        it('asks for notes once a chart exists', () => {
            expect(notes.load).toHaveBeenCalled();
        });

        it('draws notes on the current series only and lists them for keyboard readers', () => {
            notes.annotations.set([launch, campaign]);
            fixture.componentRef.setInput('comparisonData', days);
            fixture.detectChanges();

            const merge = (component as unknown as { chartMergeOptions: () => { series: { name: string; markLine?: { data: unknown[] }; markArea?: { data: unknown[] } }[] } }).chartMergeOptions();
            const current = merge.series.find((series) => series.name === 'Events');
            expect(current?.markLine?.data).toEqual([{ xAxis: 1, name: 'Launch' }]);
            expect(current?.markArea?.data).toEqual([[{ xAxis: 0, name: 'Campaign', label: { distance: 6 } }, { xAxis: 2 }]]);
            expect(merge.series.find((series) => series.name !== 'Events')?.markLine).toBeUndefined();

            const chips = fixture.debugElement.queryAll(By.css('app-annotation-strip button'));
            expect(chips.map((chip) => chip.nativeElement.textContent)).toEqual([expect.stringContaining('Campaign'), expect.stringContaining('Launch')]);
            chips[1].nativeElement.click();
            expect(notes.openExisting).toHaveBeenCalledWith(launch, 'day');
        });

        it('offers the add button only to people who can write notes', () => {
            fixture.detectChanges();
            expect(fixture.debugElement.query(By.css('p-button'))).toBeNull();

            notes.canWrite.set(true);
            fixture.detectChanges();
            fixture.debugElement.query(By.css('p-button button')).nativeElement.click();
            expect(notes.open).toHaveBeenCalledWith({ startsAt: new Date('2026-07-03T00:00:00Z'), endsAt: null, body: '', granularity: 'day' }, expect.any(HTMLButtonElement));
        });

        it('opens the note behind a clicked marker', () => {
            notes.annotations.set([launch, campaign]);
            fixture.detectChanges();
            const chart = liveChart();
            (component as unknown as { onChartInit: (chart: unknown) => void }).onChartInit(chart);

            chart.handlers.get('click')?.({ componentType: 'markArea', dataIndex: 0 });
            expect(notes.openExisting).toHaveBeenCalledWith(campaign, 'day', expect.any(HTMLButtonElement));
        });

        it('counts notes in the chart label', () => {
            notes.annotations.set([launch]);
            fixture.detectChanges();
            const label = fixture.debugElement.query(By.css('div[role="img"]')).nativeElement.getAttribute('aria-label');
            expect(label).toContain('annotations.chart.aria');
        });
    });
});
