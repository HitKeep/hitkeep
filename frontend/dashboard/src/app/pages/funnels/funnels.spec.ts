import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { ApplicationRef, signal } from '@angular/core';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { TRANSLOCO_LOCALE_CONFIG, TRANSLOCO_LOCALE_LANG_MAPPING, TranslocoLocaleService } from '@jsverse/transloco-locale';
import { Observable, of, Subject } from 'rxjs';
import { vi } from 'vitest';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';

import { AnalyticsService } from '@services/analytics.service';
import { StatsService } from '@features/analytics/services/stats.service';
import { SiteService } from '@features/sites/services/site.service';
import { Funnels } from './funnels';
import { AccessService } from '@services/access.service';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { flushAnnotations } from '@testing/annotations';
import { Funnel, FunnelSeriesPoint } from '@models/analytics.types';

describe('Funnels', () => {
    const stats = signal({
        live_visitors: 0,
        total_pageviews: 0,
        unique_sessions: 4,
        bounce_rate: 0,
        avg_session_duration: 0,
        pages_per_session: 0,
        chart_data: [],
        top_pages: [{ name: '/pricing', value: 8 }],
        top_landing_pages: [],
        top_exit_pages: [],
        top_referrers: [{ name: 'https://google.com', value: 5 }],
        top_devices: [{ name: 'Desktop', value: 7 }],
        top_countries: [{ name: 'US', value: 4 }],
        top_cities: [{ name: 'Berlin', value: 3 }],
        top_providers: [{ name: 'Hetzner Online GmbH', value: 2 }],
        top_asns: [{ name: 'AS24940 Hetzner Online GmbH', value: 2 }],
        top_languages: [],
        top_utm_campaigns: [],
        top_utm_contents: [],
        top_utm_mediums: [],
        top_utm_sources: [],
        top_utm_terms: [],
        utm_campaign_hits: 0,
        utm_content_hits: 0,
        utm_medium_hits: 0,
        utm_source_hits: 0,
        utm_term_hits: 0,
        goals: [],
        funnels: []
    });
    const statsServiceStub = {
        stats,
        isLoading: signal(false),
        currentComparisonRange: signal(null),
        comparisonRange: () => null,
        loadStats: vi.fn(),
        fetchStats: vi.fn(() => of(stats()))
    };
    const analyticsServiceStub = {
        getFunnels: vi.fn<AnalyticsService['getFunnels']>(() => of([])),
        deleteFunnel: vi.fn<AnalyticsService['deleteFunnel']>(() => of(undefined)),
        getFunnelTimeseries: vi.fn<() => Observable<FunnelSeriesPoint[]>>(() => of<FunnelSeriesPoint[]>([])),
        getFunnelStats: vi.fn(() => of(null))
    };

    const funnel: Funnel = { id: 'funnel-1', site_id: 'site-1', name: 'Signup', steps: [{ type: 'path', value: '/' }], created_at: '2026-01-01T00:00:00Z' };

    beforeEach(async () => {
        vi.clearAllMocks();
        analyticsServiceStub.getFunnels.mockReset();
        analyticsServiceStub.getFunnelTimeseries.mockReset();

        await TestBed.configureTestingModule({
            imports: [
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: { funnels: 'Funnels' },
                            common: {
                                metricGroups: {
                                    content: 'Content',
                                    acquisition: 'Acquisition',
                                    audience: 'Audience',
                                    location: 'Location',
                                    network: 'Network'
                                },
                                metrics: {
                                    topPages: 'Top pages',
                                    topSources: 'Top sources',
                                    devices: 'Devices',
                                    countries: 'Countries',
                                    cities: 'Cities',
                                    providers: 'Providers',
                                    asns: 'ASNs'
                                },
                                filters: {
                                    page: 'Page: {{value}}',
                                    source: 'Source: {{value}}',
                                    device: 'Device: {{value}}',
                                    country: 'Country: {{value}}',
                                    city: 'City: {{value}}',
                                    provider: 'Provider: {{value}}',
                                    asn: 'ASN: {{value}}'
                                }
                            },
                            funnels: {
                                kpis: {
                                    funnels: 'Funnels',
                                    entries: 'Entries',
                                    completions: 'Completions',
                                    completionRate: 'Completion rate'
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
                {
                    provide: SiteService,
                    useValue: {
                        activeSite: signal({
                            id: 'site-1',
                            user_id: 'user-1',
                            domain: 'funnels.test',
                            created_at: '2026-05-01T00:00:00Z'
                        }),
                        isLoading: signal(false)
                    }
                },
                { provide: StatsService, useValue: statsServiceStub },
                { provide: AnalyticsService, useValue: analyticsServiceStub },
                { provide: AccessService, useValue: { canSite: () => true } },
                ConfirmationService,
                provideRouter([]),
                provideHttpClient(),
                provideHttpClientTesting(),
                {
                    provide: TranslocoLocaleService,
                    useValue: {
                        langChanges$: of('en'),
                        localeChanges$: of('en'),
                        getLocale: () => 'en-US',
                        localizeNumber: (value: number) => value.toString(),
                        localizeDate: (value: Date) => value.toISOString()
                    }
                },
                { provide: TRANSLOCO_LOCALE_CONFIG, useValue: {} },
                { provide: TRANSLOCO_LOCALE_LANG_MAPPING, useValue: { en: 'en-US' } }
            ]
        }).compileComponents();
    });

    it('presents one reporting subject and exactly three funnel KPIs', async () => {
        analyticsServiceStub.getFunnels.mockReturnValueOnce(
            of([
                { site_id: 'site-1', id: 'funnel-1', name: 'Signup', steps: [{ type: 'path', value: '/' }], created_at: '2026-01-01T00:00:00Z' },
                { site_id: 'site-1', id: 'funnel-2', name: 'Checkout', steps: [{ type: 'path', value: '/cart' }], created_at: '2026-01-01T00:00:00Z' }
            ])
        );
        const component = TestBed.runInInjectionContext(() => new Funnels()) as unknown as {
            kpis: () => { label: string }[];
            subjectOptions: () => { label: string; value: string | null }[];
            selectFunnel: (id: string | null) => void;
            selectedFunnelId: () => string | null;
            trafficFunnelIds: () => string[];
            trafficEnabled: () => boolean;
        };
        expect(component.kpis().length).toBe(3);
        expect(component.subjectOptions()[0].value).toBeNull();
        expect(component.trafficEnabled()).toBe(false);
        await TestBed.inject(ApplicationRef).whenStable();
        expect(component.trafficFunnelIds()).toEqual(['funnel-1', 'funnel-2']);
        component.selectFunnel('funnel-2');
        expect(component.trafficFunnelIds()).toEqual(['funnel-2']);
        component.selectFunnel(null);
        expect(component.selectedFunnelId()).toBeNull();
    });

    it('keeps the deep-linked funnel pending until Refresh retries its definitions', async () => {
        vi.spyOn(TestBed.inject(ActivatedRoute), 'queryParamMap', 'get').mockReturnValue(of(convertToParamMap({ funnel: funnel.id })));
        const analytics = TestBed.runInInjectionContext(() => new AnalyticsService());
        analyticsServiceStub.getFunnels.mockImplementation((siteId) => analytics.getFunnels(siteId));
        const http = TestBed.inject(HttpTestingController);
        const fixture = TestBed.createComponent(Funnels);
        const page = fixture.componentInstance;
        TestBed.tick();
        page['refresh']('background');
        expect(page['trafficEnabled']()).toBe(false);
        expect(analyticsServiceStub.getFunnelTimeseries).not.toHaveBeenCalled();

        flushAnnotations(http);
        http.expectNone((req) => req.url.endsWith('/hits'));
        http.expectOne('/api/sites/site-1/funnels').flush(null, { status: 503, statusText: 'Unavailable' });
        await vi.waitFor(() => expect(fixture.nativeElement.querySelector('app-page-state button')).not.toBeNull());
        page['refresh']('background');
        expect(page['loadError']()).toBeTruthy();
        expect(page['selectedFunnelId']()).toBe(funnel.id);
        expect(analyticsServiceStub.getFunnels).toHaveBeenCalledTimes(1);
        expect(analyticsServiceStub.getFunnelStats).not.toHaveBeenCalled();
        expect(statsServiceStub.fetchStats).not.toHaveBeenCalled();

        fixture.nativeElement.querySelector('app-page-state button').click();
        TestBed.tick();
        http.expectOne('/api/sites/site-1/funnels').flush([funnel]);
        await vi.waitFor(() => expect(page['selectedFunnel']()?.id).toBe(funnel.id));
        flushAnnotations(http);
        const hits = http.expectOne((req) => req.url.endsWith('/hits'));
        expect(hits.request.params.getAll('funnel_id')).toEqual([funnel.id]);
        hits.flush({ data: [], total: 0 });
        http.verify();

        expect(analyticsServiceStub.getFunnels).toHaveBeenCalledTimes(2);
        expect(page['loadError']()).toBeFalsy();
        expect(fixture.nativeElement.querySelector('app-page-state')).toBeNull();
        expect(fixture.nativeElement.querySelectorAll('app-kpi-card').length).toBe(3);
        expect(page['subjectControl'].value).toBe(funnel.id);
        expect(page['trafficEnabled']()).toBe(true);
        expect(analyticsServiceStub.getFunnelTimeseries).toHaveBeenCalledTimes(2);
        expect(analyticsServiceStub.getFunnelStats).toHaveBeenCalledWith('site-1', funnel.id, expect.any(String), expect.any(String));
    });

    it('cancels the previous site lookup and reloads definitions after create and delete', async () => {
        const old = new Subject<Funnel[]>();
        const created = { ...funnel, site_id: 'site-2' };
        analyticsServiceStub.getFunnels
            .mockReturnValueOnce(old)
            .mockReturnValueOnce(of([]))
            .mockReturnValueOnce(of([created]))
            .mockReturnValueOnce(of([]));
        const page = TestBed.runInInjectionContext(() => new Funnels());
        TestBed.tick();
        const site = TestBed.inject(SiteService).activeSite;
        site.set({ ...site()!, id: 'site-2' });
        await TestBed.inject(ApplicationRef).whenStable();
        expect(old.observed).toBe(false);
        old.next([funnel]);
        expect(page['funnels']()).toEqual([]);

        page['onDefinitionsChanged']();
        await TestBed.inject(ApplicationRef).whenStable();
        expect(page['funnels']()).toEqual([created]);
        page['deleteFunnel'](created);
        await TestBed.inject(ApplicationRef).whenStable();
        expect(analyticsServiceStub.deleteFunnel).toHaveBeenCalledWith('site-2', created.id);
        expect(analyticsServiceStub.getFunnels.mock.calls.flat()).toEqual(['site-1', 'site-2', 'site-2', 'site-2']);
        expect(page['funnels']()).toEqual([]);
    });

    it('ignores a late funnel series response after the reporting scope changes', () => {
        const component = TestBed.runInInjectionContext(() => new Funnels()) as unknown as {
            loadReporting: (siteID: string, from: string, to: string, funnelIDs: string[]) => void;
            funnelSeries: () => { time: string; entries: number; completions: number }[];
        };
        const first = new Subject<FunnelSeriesPoint[]>();
        const second = new Subject<FunnelSeriesPoint[]>();
        analyticsServiceStub.getFunnelTimeseries.mockReset().mockReturnValue(of<FunnelSeriesPoint[]>([])).mockReturnValueOnce(first).mockReturnValueOnce(of([])).mockReturnValueOnce(second);

        component.loadReporting('site-1', '2026-07-01', '2026-07-02', ['funnel-1']);
        component.loadReporting('site-1', '2026-07-01', '2026-07-02', ['funnel-2']);
        first.next([{ time: '2026-07-01', entries: 1, completions: 1 }]);
        second.next([{ time: '2026-07-01', entries: 2, completions: 1 }]);
        second.complete();

        expect(component.funnelSeries()).toEqual([{ time: '2026-07-01', entries: 2, completions: 1 }]);
    });
});
