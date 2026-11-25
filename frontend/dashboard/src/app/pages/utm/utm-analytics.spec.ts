import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { TRANSLOCO_LOCALE_CONFIG, TRANSLOCO_LOCALE_LANG_MAPPING, TranslocoLocaleService } from '@jsverse/transloco-locale';
import { of, Subject, throwError } from 'rxjs';
import { vi } from 'vitest';

import { StatsService } from '@features/analytics/services/stats.service';
import { SiteService } from '@features/sites/services/site.service';
import { emptySiteStats } from '@testing/empty-site-stats';
import type { SiteStats } from '@models/analytics.types';
import en from '../../../../public/i18n/en.json';
import { UtmAnalytics } from './utm-analytics';

describe('UtmAnalytics', () => {
    let fixture: ComponentFixture<UtmAnalytics>;
    const stats = emptySiteStats({
        top_utm_campaigns: [{ name: 'launch', value: 12 }, ...Array.from({ length: 9 }, (_, i) => ({ name: `campaign-${i}`, value: 1 }))],
        top_utm_contents: [{ name: 'hero', value: 7 }],
        top_utm_mediums: [{ name: 'cpc', value: 8 }],
        top_utm_sources: [{ name: 'google', value: 10 }],
        top_utm_terms: [{ name: 'analytics', value: 4 }],
        utm_campaign_hits: 24,
        utm_content_hits: 7,
        utm_medium_hits: 8,
        utm_source_hits: 10,
        utm_term_hits: 4,
        paid_signal_sessions: 4,
        paid_signal_measured_sessions: 15
    });
    const statsServiceStub = {
        fetchStats: vi.fn(() => of(stats))
    };
    const paidSignalsCard = () => fixture.nativeElement.querySelector('[data-testid="utm-paid-signals"]') as HTMLElement;

    beforeEach(async () => {
        statsServiceStub.fetchStats.mockReset();

        await TestBed.configureTestingModule({
            imports: [
                UtmAnalytics,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: { utm: 'UTM' },
                            common: {
                                loading: 'Loading...',
                                loadingSiteData: 'Loading site data...',
                                noSiteSelected: 'No site selected',
                                noActiveFilter: 'No active filter',
                                removeFilterAria: 'Remove filter',
                                selectDateRange: 'Select date range',
                                actions: { apply: 'Apply', cancel: 'Cancel', clearAll: 'Clear all' },
                                metricGroups: { acquisition: 'Acquisition' }
                            },
                            dashboard: {
                                kpis: { pageviews: 'Pageviews' },
                                traffic: { visitors: 'Visitors' }
                            },
                            utm: {
                                paidSignals: en.utm.paidSignals,
                                kpis: {
                                    campaign: 'Campaign',
                                    content: 'Content',
                                    medium: 'Medium',
                                    source: 'Source',
                                    term: 'Term'
                                },
                                metrics: {
                                    topCampaigns: 'Campaigns',
                                    topSources: 'Sources',
                                    topMediums: 'Mediums',
                                    topContents: 'Contents',
                                    topTerms: 'Terms'
                                },
                                filters: {
                                    campaign: 'Campaign: {{value}}',
                                    content: 'Content: {{value}}',
                                    medium: 'Medium: {{value}}',
                                    source: 'Source: {{value}}',
                                    term: 'Term: {{value}}'
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
                provideRouter([]),
                {
                    provide: SiteService,
                    useValue: {
                        activeSite: signal({
                            id: 'site-1',
                            user_id: 'user-1',
                            domain: 'utm.test',
                            created_at: '2026-05-01T00:00:00Z'
                        }),
                        isLoading: signal(false)
                    }
                },
                { provide: StatsService, useValue: statsServiceStub },
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

        fixture = TestBed.createComponent(UtmAnalytics);
        await fixture.whenStable();
    });

    it('keeps the UTM metric cards and uses their filters to reload paid signal counts', async () => {
        const component = fixture.componentInstance as unknown as {
            metricCardTabs: () => { id: string; cards: { id: string; linkMode?: string }[] }[];
            onMetricCardClick: (event: { tabId: string; cardId: string; filterType: string; metric: { name: string; value: number } }) => void;
            activeFilters: () => { type: string; value: string }[];
            activeFilterValue: (type: 'utm_source') => string | null;
        };

        const tabs = component.metricCardTabs();

        expect(fixture.nativeElement.querySelector('.metric-list__share')?.textContent.trim()).toBe('50%');

        expect(tabs.map((tab) => tab.id)).toEqual(['acquisition']);
        expect(tabs[0]?.cards.map((card) => card.id)).toEqual(['campaigns', 'sources', 'mediums', 'contents', 'terms']);
        expect(tabs[0]?.cards.find((card) => card.id === 'sources')?.linkMode).toBeUndefined();
        expect(paidSignalsCard().textContent).toContain('4 of 15 measured');
        expect(paidSignalsCard().textContent).toContain(en.utm.paidSignals.scope);

        const response = new Subject<SiteStats>();
        statsServiceStub.fetchStats.mockReturnValueOnce(response);
        component.onMetricCardClick({ tabId: 'acquisition', cardId: 'sources', filterType: 'utm_source', metric: { name: 'google', value: 10 } });
        TestBed.tick();

        expect(component.activeFilters()).toEqual([{ type: 'utm_source', value: 'google' }]);
        expect(component.activeFilterValue('utm_source')).toBe('google');
        expect(statsServiceStub.fetchStats).toHaveBeenLastCalledWith('site-1', expect.any(String), expect.any(String), [{ type: 'utm_source', value: 'google' }], [], [], 'utm');
        expect(paidSignalsCard().textContent).toContain('Loading...');
        expect(paidSignalsCard().textContent).not.toContain('4 of 15 measured');

        response.next(emptySiteStats({ paid_signal_sessions: 0, paid_signal_measured_sessions: 3 }));
        response.complete();
        await fixture.whenStable();

        expect(fixture.nativeElement.querySelector('.metric-list__empty-label')).toBeTruthy();
        expect(paidSignalsCard().textContent).toContain('0 of 3 measured');
        expect(paidSignalsCard().textContent).toContain(en.utm.paidSignals.unobserved);
        expect(paidSignalsCard().textContent).toContain(en.utm.paidSignals.unobservedHint);
        expect(paidSignalsCard().querySelectorAll('dd')[1]?.textContent?.trim()).toBe('3');
    });

    it('distinguishes zero measured sessions from unavailable stats', async () => {
        const component = fixture.componentInstance as unknown as { refreshStats: () => void };
        statsServiceStub.fetchStats.mockReturnValueOnce(of(emptySiteStats({ paid_signal_sessions: 0, paid_signal_measured_sessions: 0 })));
        component.refreshStats();
        await fixture.whenStable();

        expect(paidSignalsCard().textContent).toContain(en.utm.paidSignals.noMeasurement);
        expect(paidSignalsCard().querySelector('dl')).toBeNull();

        const log = vi.spyOn(console, 'error').mockImplementation(() => undefined);
        statsServiceStub.fetchStats.mockReturnValueOnce(throwError(() => new Error('unavailable')));
        component.refreshStats();
        await fixture.whenStable();

        expect(paidSignalsCard().textContent).toContain(en.utm.paidSignals.unavailable);
        expect(paidSignalsCard().textContent).not.toContain(en.utm.paidSignals.noMeasurement);
        log.mockRestore();

        component.refreshStats();
        await fixture.whenStable();
        expect(paidSignalsCard().textContent).toContain('4 of 15 measured');
    });
});
