import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { signal } from '@angular/core';
import { of, Subject, throwError } from 'rxjs';
import { vi } from 'vitest';
import { By } from '@angular/platform-browser';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { TRANSLOCO_LOCALE_CONFIG, TRANSLOCO_LOCALE_LANG_MAPPING, TranslocoLocaleService } from '@jsverse/transloco-locale';
import { WebVitalsPage } from './web-vitals';
import { SiteService } from '@features/sites/services/site.service';
import { AnalyticsService } from '@core/services/analytics.service';
import { SeriesChart } from '@features/analytics/components/series-chart';
import type { WebVitalDimensionRow, WebVitalSeriesPoint } from '@models/analytics.types';
import type { SiteSetupState } from '@services/setup-state.service';
import { flushAnnotations } from '@testing/annotations';
import { PreferenceStorage } from '@services/preference-storage';
import { ReportRangePreferencesService } from '@services/report-range-preferences.service';
import { RealtimeRefreshCoordinator } from '@services/realtime-refresh-coordinator.service';
import { flushSetupState } from '@testing/setup-state';

describe('WebVitalsPage', () => {
    let fixture: ComponentFixture<WebVitalsPage>;
    let httpMock: HttpTestingController;
    const mockAnalytics = (service: AnalyticsService) => ({
        getWebVitalsSummary: vi.spyOn(service, 'getWebVitalsSummary').mockImplementation(() =>
            of([
                { metric: 'LCP', p75: 2400, samples: 120, good: 90, needs_improvement: 20, poor: 10, rating: 'good' },
                { metric: 'INP', p75: 280, samples: 90, good: 50, needs_improvement: 30, poor: 10, rating: 'needs_improvement' },
                { metric: 'CLS', p75: 0.09, samples: 88, good: 70, needs_improvement: 12, poor: 6, rating: 'good' }
            ])
        ),
        getWebVitalsTimeseries: vi.spyOn(service, 'getWebVitalsTimeseries').mockImplementation(() => of([{ time: '2026-05-06T00:00:00Z', p75: 2400, samples: 24, good: 18, needs_improvement: 4, poor: 2 }])),
        getWebVitalsPages: vi.spyOn(service, 'getWebVitalsPages').mockImplementation(() =>
            of([
                {
                    path: '/pricing',
                    p75: 3100,
                    samples: 24,
                    good: 12,
                    needs_improvement: 8,
                    poor: 4,
                    rating: 'needs_improvement',
                    metrics: {
                        LCP: { p75: 3100, samples: 24, good: 12, needs_improvement: 8, poor: 4, rating: 'needs_improvement' },
                        CLS: { p75: 0.08, samples: 24, good: 21, needs_improvement: 2, poor: 1, rating: 'good' }
                    }
                }
            ])
        ),
        getWebVitalsBreakdown: vi.spyOn(service, 'getWebVitalsBreakdown').mockImplementation(() => of([{ name: 'Chrome', p75: 2800, samples: 40, good: 30, needs_improvement: 7, poor: 3, rating: 'needs_improvement' }]))
    });
    let analyticsServiceMock: ReturnType<typeof mockAnalytics>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                WebVitalsPage,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: { webVitals: 'Web Vitals' },
                            common: {
                                loadingSiteData: 'Loading site data...',
                                noSiteSelected: 'No site selected',
                                selectDateRange: 'Select date range',
                                searchPlaceholder: 'Search...',
                                columns: { actions: 'Actions' },
                                actions: { retry: 'Retry', apply: 'Apply', cancel: 'Cancel', clearAll: 'Clear all' },
                                removeFilterAria: 'Remove filter',
                                timeRanges: {
                                    today: 'Today',
                                    todayShort: 'Today',
                                    yesterday: 'Yesterday',
                                    yesterdayShort: 'Yesterday',
                                    lastMinute: 'Last minute',
                                    lastMinutes: 'Last {{count}} minutes',
                                    lastHour: 'Last hour',
                                    lastHours: 'Last {{count}} hours',
                                    lastDay: 'Last day',
                                    lastDays: 'Last {{count}} days',
                                    lastYear: 'Last year',
                                    lastYearShort: '1y',
                                    allRanges: 'All ranges',
                                    customRange: 'Custom range',
                                    customShort: 'Custom',
                                    customRangeSummary: '{{start}} - {{end}}'
                                },
                                empty: { noDataTitle: 'No data yet', noDataDescription: 'No data.' },
                                seriesChartAria: 'Series chart with {{count}} points'
                            },
                            webVitals: {
                                docsAction: 'Setup guide',
                                error: 'Web Vitals data could not be loaded.',
                                noSiteDescription: 'Select a site.',
                                metrics: { LCP: 'LCP', INP: 'INP', CLS: 'CLS', FCP: 'FCP', TTFB: 'TTFB' },
                                ratings: { good: 'Good', needs_improvement: 'Needs work', poor: 'Poor', none: 'No samples' },
                                cards: { samples: '{{count}} samples', selectMetricAria: 'Inspect {{metric}} over time', goodUntil: 'Good to {{value}}' },
                                filters: {
                                    metric: 'Metric',
                                    rating: 'Rating',
                                    path: 'Page path',
                                    allRatings: 'All ratings',
                                    ratingChip: 'Rating: {{rating}}',
                                    pathChip: 'Path: {{path}}'
                                },
                                chart: { title: '{{metric}} p75 trend', description: 'p75 over time', p75: '{{metric}} p75' },
                                distribution: { title: 'Rating mix', description: 'Site-wide {{metric}} samples.' },
                                pages: {
                                    title: 'Page breakdown',
                                    description: 'All paths with {{metric}} samples.',
                                    filteredTitle: '{{rating}} {{metric}} pages',
                                    filteredDescription: 'Showing paths that produced {{rating}} {{metric}} samples.',
                                    empty: 'No Web Vitals samples match the current filters.'
                                },
                                breakdown: {
                                    title: '{{dimension}} breakdown',
                                    description: '{{metric}} by visitor context',
                                    empty: 'No breakdown rows match the current filters.',
                                    unknown: 'Unknown',
                                    tabs: { pages: 'Pages', countries: 'Countries', languages: 'Languages', browsers: 'Browsers', devices: 'Devices' }
                                },
                                columns: { path: 'Path', p75: 'p75', samples: 'Samples', ratingSamples: '{{rating}} samples', rating: 'Rating' },
                                empty: { title: 'No Web Vitals yet', description: 'Enable Web Vitals.' },
                                setup: {
                                    title: 'Turn on Web Vitals collection',
                                    description: 'Enable Web Vitals in your tracking snippet to fill this page.',
                                    docsAction: 'Read the Web Vitals guide'
                                }
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    }
                })
            ],
            providers: [
                provideHttpClient(),
                provideHttpClientTesting(),
                provideRouter([]),
                {
                    provide: SiteService,
                    useValue: {
                        activeSite: signal({
                            id: 'site-1',
                            user_id: 'user-1',
                            domain: 'example.test',
                            created_at: '2026-05-01T00:00:00Z'
                        }),
                        isLoading: signal(false)
                    }
                },
                AnalyticsService,
                {
                    provide: TranslocoLocaleService,
                    useValue: {
                        langChanges$: of('en'),
                        localeChanges$: of('en'),
                        getLocale: () => 'en-US',
                        localizeNumber: (value: number, _style: string, _locale?: string, options?: Intl.NumberFormatOptions) => new Intl.NumberFormat('en-US', options).format(value),
                        localizeDate: (value: Date) => value.toISOString()
                    }
                },
                { provide: TRANSLOCO_LOCALE_CONFIG, useValue: {} },
                { provide: TRANSLOCO_LOCALE_LANG_MAPPING, useValue: { en: 'en-US' } }
            ]
        }).compileComponents();

        analyticsServiceMock = mockAnalytics(TestBed.inject(AnalyticsService));
        httpMock = TestBed.inject(HttpTestingController);
        fixture = TestBed.createComponent(WebVitalsPage);
        fixture.detectChanges();
    });

    afterEach(() => {
        // Drain the shared setup-state and chart-notes lookups for the tests that do not assert on them.
        flushSetupState(httpMock, 'site-1');
        flushAnnotations(httpMock);
        httpMock.verify();
    });

    /** Recreates the page on a range without a single Web Vitals sample. */
    const createWithoutSamples = (): void => {
        analyticsServiceMock.getWebVitalsSummary.mockReturnValue(of([]));
        analyticsServiceMock.getWebVitalsTimeseries.mockReturnValue(of([{ time: '2026-05-06T00:00:00Z', p75: 0, samples: 0, good: 0, needs_improvement: 0, poor: 0 }]));
        analyticsServiceMock.getWebVitalsPages.mockReturnValue(of([]));
        analyticsServiceMock.getWebVitalsBreakdown.mockReturnValue(of([]));
        fixture = TestBed.createComponent(WebVitalsPage);
        fixture.detectChanges();
    };

    const answerSetupState = (overrides: Partial<SiteSetupState> = {}) => flushSetupState(httpMock, 'site-1', overrides, fixture);

    it('renders a setup callout when the site never reported a Web Vital', () => {
        createWithoutSamples();
        answerSetupState({ has_web_vitals: false });

        const callout = fixture.nativeElement.querySelector('[data-testid="web-vitals-setup-callout"]');
        expect(callout).not.toBeNull();
        expect(callout.textContent).toContain('Turn on Web Vitals collection');
        expect(callout.textContent).toContain('Enable Web Vitals in your tracking snippet to fill this page.');
        expect(callout.querySelector('a[href="https://hitkeep.com/guides/analytics/web-vitals/"]')).not.toBeNull();
        expect(fixture.nativeElement.textContent).not.toContain('Page breakdown');
        expect(fixture.nativeElement.querySelector('app-report-range-toolbar')).not.toBeNull();
    });

    it('keeps the regular empty states when samples exist outside the selected range', () => {
        createWithoutSamples();
        answerSetupState({ has_web_vitals: true });

        expect(fixture.nativeElement.querySelector('[data-testid="web-vitals-setup-callout"]')).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('Page breakdown');
        expect(fixture.nativeElement.querySelector('app-series-chart [echarts]')).toBeNull();
    });

    it('never shows the callout while the setup state is still unknown', () => {
        createWithoutSamples();

        httpMock.expectOne('/api/sites/site-1/setup-state');
        expect(fixture.nativeElement.querySelector('[data-testid="web-vitals-setup-callout"]')).toBeNull();
    });

    it('skips the setup-state lookup while the selected range has samples', () => {
        httpMock.expectNone('/api/sites/site-1/setup-state');
    });

    it('renders populated Web Vitals report sections', () => {
        const text = fixture.nativeElement.textContent;
        expect(text).toContain('LCP p75 trend');
        expect(text).toContain('Rating mix');
        expect(text).toContain('Page breakdown');
        expect(text).toContain('/pricing');
        const timeseriesCall = analyticsServiceMock.getWebVitalsTimeseries.mock.calls[0];
        expect(timeseriesCall[0]).toBe('site-1');
        expect(timeseriesCall[3]).toBe('LCP');
        expect(timeseriesCall[4]).toBe('/');
        expect(timeseriesCall[5]).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('Path: /');
    });

    it('keeps missing p75 buckets distinct from sampled zero and uses the selected metric units', () => {
        const chart: SeriesChart = fixture.debugElement.query(By.directive(SeriesChart)).componentInstance;
        expect(chart.valueFormatter()?.(2400)).toBe('2.4 s');
        expect(chart.data()[0]['p75']).toBe(2400);

        const pendingLcp = new Subject<WebVitalSeriesPoint[]>();
        const cls = new Subject<WebVitalSeriesPoint[]>();
        analyticsServiceMock.getWebVitalsTimeseries.mockReturnValueOnce(pendingLcp).mockReturnValue(cls);
        fixture.debugElement.query(By.css('app-report-range-toolbar')).triggerEventHandler('refresh');
        TestBed.tick();
        const lcpCard = fixture.nativeElement.querySelector('[aria-label^="Inspect LCP over time"]') as HTMLElement;
        const clsCard = fixture.nativeElement.querySelector('[aria-label^="Inspect CLS over time"]') as HTMLElement;
        expect(lcpCard.getAttribute('aria-pressed')).toBe('true');
        expect(clsCard.getAttribute('aria-pressed')).toBe('false');
        clsCard.click();
        fixture.detectChanges();
        expect(lcpCard.getAttribute('aria-pressed')).toBe('false');
        expect(clsCard.getAttribute('aria-pressed')).toBe('true');
        expect(pendingLcp.observed).toBe(false);
        expect(chart.data()).toEqual([]);

        const measuredZero = { time: '2026-05-06T00:00:00Z', p75: 0, samples: 1, good: 1, needs_improvement: 0, poor: 0 };
        const noSamples = { ...measuredZero, time: '2026-05-07T00:00:00Z', samples: 0, good: 0 };
        cls.next([measuredZero, noSamples]);
        cls.complete();
        fixture.detectChanges();

        expect(chart.data()).toEqual([
            { time: measuredZero.time, p75: 0 },
            { time: noSamples.time, p75: null }
        ]);
        expect(chart.valueFormatter()?.(0.08)).toBe('0.08');
        expect(fixture.nativeElement.querySelector('app-series-chart [echarts]')).not.toBeNull();
    });

    it('exposes rating filter selection and deselection through pressed state', () => {
        const ratingButtons = Array.from<HTMLButtonElement>(fixture.nativeElement.querySelectorAll('.web-vitals-dist-cell'));
        const pressedStates = () => ratingButtons.map((button) => button.getAttribute('aria-pressed'));
        expect(pressedStates()).toEqual(['false', 'false', 'false']);

        ratingButtons[1].click();
        fixture.detectChanges();
        expect(pressedStates()).toEqual(['false', 'true', 'false']);
        expect(analyticsServiceMock.getWebVitalsTimeseries.mock.calls.at(-1)?.[5]).toBe('needs_improvement');

        ratingButtons[1].click();
        fixture.detectChanges();
        expect(pressedStates()).toEqual(['false', 'false', 'false']);
        expect(analyticsServiceMock.getWebVitalsTimeseries.mock.calls.at(-1)?.[5]).toBeNull();
    });

    it('keeps breakdown tables searchable, sortable, and paginated', () => {
        const assertTableControls = (selector: string, columns: number) => {
            const table = fixture.debugElement.query(By.css(selector));
            expect(table.nativeElement.querySelector('input[placeholder="Search..."]')).not.toBeNull();
            expect(table.query(By.css('p-table')).componentInstance.paginator).toBe(true);
            expect(table.nativeElement.querySelectorAll('p-sorticon').length).toBeGreaterThanOrEqual(columns);
        };
        assertTableControls('[data-testid="web-vitals-pages-table"]', 8);
        const devices = Array.from<HTMLElement>(fixture.nativeElement.querySelectorAll('[role="tab"]')).find((tab) => tab.textContent.trim() === 'Devices')!;
        devices.click();
        TestBed.tick();
        assertTableControls('[role="tabpanel"]:not([hidden]) [data-testid="web-vitals-breakdown-table"]', 4);
    });

    it('keeps metric cards site-wide while passing rating and path filters to drilldowns', () => {
        const component = fixture.componentInstance as unknown as {
            selectedRating: { set: (value: string) => void };
            selectPathFilter: (value: string) => void;
        };
        component.selectedRating.set('poor');
        component.selectPathFilter('/checkout');
        fixture.detectChanges();

        const summaryCall = analyticsServiceMock.getWebVitalsSummary.mock.calls.at(-1);
        const pagesCall = analyticsServiceMock.getWebVitalsPages.mock.calls.at(-1);
        expect(summaryCall?.[0]).toBe('site-1');
        expect(summaryCall?.[3]).toBeNull();
        expect(summaryCall?.[4]).toBeNull();
        expect(summaryCall?.[5]).toBeNull();
        expect(pagesCall?.[0]).toBe('site-1');
        expect(pagesCall?.[3]).toBe('LCP');
        expect(pagesCall?.[4]).toBeNull();
        expect(pagesCall?.[5]).toBe('poor');
        expect(pagesCall?.[6]).toBe(100);
        expect(fixture.nativeElement.textContent).toContain('Poor LCP pages');
        expect(fixture.nativeElement.textContent).toContain('Poor samples');
        expect(fixture.nativeElement.textContent).toContain('4');
    });

    it('resets optional filters back to the homepage Web Vitals scope', () => {
        const component = fixture.componentInstance as unknown as {
            selectedRating: { set: (value: string) => void };
            selectPathFilter: (value: string) => void;
            clearAllFilters: () => void;
        };
        component.selectedRating.set('poor');
        component.selectPathFilter('/pricing?campaign=spring#plans');
        component.clearAllFilters();
        fixture.detectChanges();

        const summaryCall = analyticsServiceMock.getWebVitalsSummary.mock.calls.at(-1);
        const pagesCall = analyticsServiceMock.getWebVitalsPages.mock.calls.at(-1);
        expect(summaryCall?.[4]).toBeNull();
        expect(summaryCall?.[5]).toBeNull();
        expect(pagesCall?.[4]).toBeNull();
        expect(pagesCall?.[5]).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('Path: /');
    });

    it('loads three report reads on Pages and only the selected dimension through keyboard or pointer', () => {
        const counts = () => Object.values(analyticsServiceMock).map((read) => read.mock.calls.length);
        const tabs = Array.from<HTMLElement>(fixture.nativeElement.querySelectorAll('[role="tab"]'));
        const device = tabs.find((tab) => tab.textContent.trim() === 'Devices')!;
        expect(counts()).toEqual([1, 1, 1, 0]);
        tabs[0].focus();
        tabs[0].dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', code: 'ArrowRight', bubbles: true }));
        expect(document.activeElement).toBe(device);
        device.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', code: 'Enter', bubbles: true }));
        TestBed.tick();
        expect(device.getAttribute('aria-selected')).toBe('true');
        expect(counts()).toEqual([1, 1, 1, 1]);
        expect(analyticsServiceMock.getWebVitalsBreakdown.mock.lastCall?.[4]).toBe('device');

        tabs.find((tab) => tab.textContent.trim() === 'Countries')!.click();
        TestBed.tick();
        expect(counts()).toEqual([1, 1, 1, 2]);
        expect(analyticsServiceMock.getWebVitalsBreakdown.mock.lastCall?.[4]).toBe('country');

        tabs[0].click();
        TestBed.tick();
        expect(counts()).toEqual([1, 1, 1, 2]);
        expect(fixture.nativeElement.textContent).toContain('/pricing');
    });

    it('cancels obsolete breakdown HTTP requests on tab, site and range changes and refreshes the active scope', async () => {
        flushAnnotations(httpMock);
        fixture.destroy();
        Object.values(analyticsServiceMock).forEach((read) => read.mockRestore());
        const ranges = TestBed.inject(ReportRangePreferencesService);
        vi.spyOn(TestBed.inject(PreferenceStorage), 'write').mockReturnValue(undefined);
        ranges.selectRange({ value: { value: 'custom' }, customRange: [new Date(2026, 4, 1), new Date(2026, 4, 2)] });
        const realtime = vi.spyOn(TestBed.inject(RealtimeRefreshCoordinator), 'registerSignalUntilDestroyed');
        fixture = TestBed.createComponent(WebVitalsPage);
        TestBed.tick();
        const endpoint = (site: string, name: string) => `/api/sites/${site}/web-vitals/${name}`;
        const readReport = (site: string) => {
            for (const name of ['summary', 'timeseries', 'pages']) {
                const read = httpMock.expectOne((req) => req.url === endpoint(site, name));
                expect(read.request.params.get('from')).toBe(ranges.currentDateRange()!.from);
                expect(read.request.params.get('to')).toBe(ranges.currentDateRange()!.to);
                read.flush(name === 'summary' ? [{ metric: 'LCP', p75: 2400, samples: 120, rating: 'good' }] : []);
            }
        };
        const readDimension = (site: string, dimension: string) => {
            const read = httpMock.expectOne((req) => req.url === endpoint(site, 'breakdown'));
            expect(read.request.params.get('from')).toBe(ranges.currentDateRange()!.from);
            expect(read.request.params.get('to')).toBe(ranges.currentDateRange()!.to);
            expect(read.request.params.get('dimension')).toBe(dimension);
            return read;
        };
        const page = fixture.componentInstance;
        readReport('site-1');
        httpMock.expectNone((req) => req.url.includes('/web-vitals/'));

        page['setBreakdownTab']('device');
        TestBed.tick();
        const device = readDimension('site-1', 'device');
        page['setBreakdownTab']('country');
        TestBed.tick();
        expect(device.cancelled).toBe(true);
        const firstSite = readDimension('site-1', 'country');
        flushAnnotations(httpMock);
        const site = TestBed.inject(SiteService).activeSite;
        site.set({ ...site()!, id: 'site-2' });
        TestBed.tick();
        expect(firstSite.cancelled).toBe(true);
        readReport('site-2');
        const firstRange = readDimension('site-2', 'country');
        ranges.selectRange({ value: { value: 'custom' }, customRange: [new Date(2026, 5, 1), new Date(2026, 5, 2)] });
        TestBed.tick();
        expect(firstRange.cancelled).toBe(true);
        readReport('site-2');
        readDimension('site-2', 'country').flush([{ name: 'Canada', p75: 2200, samples: 2, rating: 'good' }]);
        flushAnnotations(httpMock);
        await fixture.whenStable();
        expect(fixture.nativeElement.querySelector('[role="tabpanel"]:not([hidden]) [data-testid="web-vitals-breakdown-table"]').textContent).toContain('Canada');

        for (const refresh of [() => page['refreshData'](), () => realtime.mock.calls[0][1].signal.update((value) => value + 1)]) {
            refresh();
            TestBed.tick();
            readReport('site-2');
            const pending = readDimension('site-2', 'country');
            expect(pending.cancelled).toBe(false);
            pending.flush([]);
        }
        page['setBreakdownTab']('pages');
        TestBed.tick();
        httpMock.expectNone((req) => req.url.includes('/web-vitals/'));
    });

    it('retries a failed dimension inside its panel while keeping the chart, KPIs and filters available', async () => {
        const pending = new Subject<WebVitalDimensionRow[]>();
        analyticsServiceMock.getWebVitalsBreakdown.mockReturnValueOnce(pending);
        fixture.componentInstance['setBreakdownTab']('browser');
        TestBed.tick();
        expect(fixture.debugElement.query(By.css('[role="tabpanel"]:not([hidden]) [data-testid="web-vitals-breakdown-table"]')).componentInstance.loading()).toBe(true);
        expect(fixture.nativeElement.querySelector('app-series-chart [echarts]')).not.toBeNull();
        pending.error(new Error('Unavailable'));
        flushAnnotations(httpMock);
        await fixture.whenStable();
        const alert = fixture.nativeElement.querySelector('[role="tabpanel"] [role="alert"]');
        expect(alert.textContent).toContain('Web Vitals data could not be loaded.');
        expect(fixture.nativeElement.querySelector('.web-vitals-card--selected').textContent).toContain('2.4 s');
        expect(fixture.nativeElement.querySelector('app-series-chart [echarts]')).not.toBeNull();
        expect(fixture.nativeElement.querySelector('#web-vitals-rating-select')).not.toBeNull();
        const retry = alert.parentElement.querySelector('button');
        expect(retry.textContent).toContain('Retry');
        retry.click();
        await fixture.whenStable();
        expect(fixture.nativeElement.querySelector('[role="alert"]')).toBeNull();
        expect(fixture.nativeElement.querySelector('[role="tabpanel"]:not([hidden]) [data-testid="web-vitals-breakdown-table"]').textContent).toContain('Chrome');
        expect(analyticsServiceMock.getWebVitalsBreakdown).toHaveBeenCalledTimes(2);
        expect(analyticsServiceMock.getWebVitalsSummary).toHaveBeenCalledTimes(1);
        expect(analyticsServiceMock.getWebVitalsTimeseries).toHaveBeenCalledTimes(1);
        expect(analyticsServiceMock.getWebVitalsPages).toHaveBeenCalledTimes(1);
    });

    it('renders an error state when the report fails', () => {
        analyticsServiceMock.getWebVitalsSummary.mockReturnValue(throwError(() => new Error('nope')));
        fixture = TestBed.createComponent(WebVitalsPage);
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Web Vitals data could not be loaded.');
    });
});
