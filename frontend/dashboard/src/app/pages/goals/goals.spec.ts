import { TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { TRANSLOCO_LOCALE_CONFIG, TRANSLOCO_LOCALE_LANG_MAPPING, TranslocoLocaleService } from '@jsverse/transloco-locale';
import { of, Subject, throwError } from 'rxjs';
import { vi } from 'vitest';
import { provideRouter } from '@angular/router';

import { AnalyticsService } from '@services/analytics.service';
import { StatsService } from '@features/analytics/services/stats.service';
import { SiteService } from '@features/sites/services/site.service';
import { Goals } from './goals';
import { AccessService } from '@services/access.service';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { Goal, GoalSeriesPoint, SiteStats } from '@models/analytics.types';
import { emptySiteStats } from '@testing/empty-site-stats';
import { previousPeriod } from '@core/analytics/comparison-period';
import type { KpiComparisonCard } from '@features/analytics/components/kpi-comparison-metric';

interface GoalsReporting {
    kpis: () => KpiComparisonCard[];
    loadReporting: (siteId: string, from: string, to: string, cohortIds: string[], seriesIds: string[]) => void;
    pathSuggestions: () => string[];
    goalsError: () => boolean;
    goalSeries: () => GoalSeriesPoint[];
    comparisonGoalSeries: () => GoalSeriesPoint[];
    refresh: (mode?: 'blocking' | 'background') => void;
    deleteGoal: (goal: Goal) => void;
    onDefinitionsChanged: () => void;
}

describe('Goals', () => {
    const stats = emptySiteStats({
        unique_sessions: 4,
        top_pages: [{ name: '/pricing', value: 8 }]
    });
    const statsServiceStub = {
        fetchStats: vi.fn<StatsService['fetchStats']>(() => of(stats))
    };
    const analyticsServiceStub = {
        getGoals: vi.fn<AnalyticsService['getGoals']>(() => of([])),
        getGoalTimeseries: vi.fn<AnalyticsService['getGoalTimeseries']>(() => of([])),
        deleteGoal: vi.fn<AnalyticsService['deleteGoal']>(() => of(undefined))
    };
    const goal: Goal = { id: 'goal-1', site_id: 'site-1', name: 'Pricing', type: 'path', value: '/pricing', created_at: '2026-01-01T00:00:00Z' };
    const createReporting = () => TestBed.runInInjectionContext(() => new Goals()) as unknown as GoalsReporting;

    beforeEach(async () => {
        statsServiceStub.fetchStats.mockReset();
        analyticsServiceStub.getGoals.mockReset();
        analyticsServiceStub.getGoalTimeseries.mockReset();
        analyticsServiceStub.deleteGoal.mockClear();

        await TestBed.configureTestingModule({
            imports: [
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: { goals: 'Goals' },
                            common: {
                                kpis: { conversionRate: 'Conversion rate' },
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
                            dashboard: { kpis: { uniqueSessions: 'Unique sessions' } },
                            goals: {
                                kpis: {
                                    totalGoals: 'Goals',
                                    conversions: 'Conversions'
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
                            domain: 'goals.test',
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

    it('presents one reporting subject and exactly three conversion KPIs', () => {
        const component = TestBed.runInInjectionContext(() => new Goals()) as unknown as {
            kpis: () => { label: string }[];
            subjectOptions: () => { label: string; value: string | null }[];
            selectGoal: (id: string | null) => void;
            selectedGoalId: () => string | null;
            goals: { set: (goals: { id: string; name: string; type: 'path'; value: string; created_at: string }[]) => void };
            trafficGoalIds: () => string[];
            trafficEnabled: () => boolean;
        };
        expect(component.kpis().length).toBe(3);
        expect(component.subjectOptions()[0].value).toBeNull();
        expect(component.trafficEnabled()).toBe(false);
        component.goals.set([
            { id: 'goal-1', name: 'Pricing', type: 'path', value: '/pricing', created_at: '2026-01-01T00:00:00Z' },
            { id: 'goal-2', name: 'Checkout', type: 'path', value: '/checkout', created_at: '2026-01-01T00:00:00Z' }
        ]);
        expect(component.trafficGoalIds()).toEqual(['goal-1', 'goal-2']);
        component.selectGoal('goal-2');
        expect(component.trafficGoalIds()).toEqual(['goal-2']);
        component.selectGoal(null);
        expect(component.selectedGoalId()).toBeNull();
    });

    it('ignores a late goal series response after the reporting scope changes', () => {
        const component = TestBed.runInInjectionContext(() => new Goals()) as unknown as {
            loadReporting: (siteID: string, from: string, to: string, cohortIDs: string[], seriesIDs: string[]) => void;
            goalSeries: () => { time: string; conversions: number }[];
        };
        const first = new Subject<GoalSeriesPoint[]>();
        const second = new Subject<GoalSeriesPoint[]>();
        analyticsServiceStub.getGoalTimeseries.mockReturnValueOnce(first).mockReturnValueOnce(of([])).mockReturnValueOnce(second);

        component.loadReporting('site-1', '2026-07-01', '2026-07-02', ['goal-1'], ['goal-1']);
        component.loadReporting('site-1', '2026-07-01', '2026-07-02', ['goal-2'], ['goal-2']);
        first.next([{ time: '2026-07-01', conversions: 1 }]);
        second.next([{ time: '2026-07-01', conversions: 2 }]);
        second.complete();

        expect(component.goalSeries()).toEqual([{ time: '2026-07-01', conversions: 2 }]);
    });

    it('loads only baseline suggestions for an empty goal cohort on entry and refresh', () => {
        const component = createReporting();
        TestBed.tick();

        expect(component.kpis().map((kpi) => kpi.value)).toEqual([0, 0, 0]);
        expect(component.pathSuggestions()).toEqual(['/pricing']);
        expect(statsServiceStub.fetchStats).toHaveBeenCalledTimes(1);

        component.refresh();
        expect(statsServiceStub.fetchStats).toHaveBeenCalledTimes(2);
        component.refresh('background');
        expect(statsServiceStub.fetchStats).toHaveBeenCalledTimes(3);
        expect(statsServiceStub.fetchStats.mock.calls.every((args) => args.length === 3)).toBe(true);
        expect(analyticsServiceStub.getGoalTimeseries).not.toHaveBeenCalled();
    });

    it('keeps the session baselines when the previous conversion series fails', () => {
        const component = createReporting();
        const current = { from: '2026-07-01T00:00:00.000Z', to: '2026-07-02T23:59:59.999Z' };
        const previous = previousPeriod(current.from, current.to);
        const comparison = { ...stats, total_conversions: 0 };
        statsServiceStub.fetchStats.mockReturnValueOnce(of({ ...stats, unique_sessions: 2, comparison: { ...comparison, unique_sessions: 0 } })).mockReturnValueOnce(of({ ...stats, comparison: { ...comparison, unique_sessions: 8 } }));
        analyticsServiceStub.getGoalTimeseries.mockReturnValueOnce(of([{ time: current.from, conversions: 2 }])).mockReturnValueOnce(throwError(() => new Error('Previous series unavailable')));

        component.loadReporting('site-1', current.from, current.to, [goal.id], [goal.id]);

        expect(component.kpis().map((card) => card.value)).toEqual([2, 2, 50]);
        expect(component.kpis().map((card) => card.previous)).toEqual([null, { period: previous, value: 0 }, { period: previous, value: 0 }]);
    });

    it('preserves the definitions error without loading an empty report', () => {
        const definitions = new Subject<Goal[]>();
        analyticsServiceStub.getGoals.mockReturnValueOnce(definitions);
        const component = createReporting();
        TestBed.tick();

        component.refresh('background');
        expect(statsServiceStub.fetchStats).not.toHaveBeenCalled();
        definitions.error(new Error('Definitions unavailable'));
        TestBed.tick();
        component.refresh('background');

        expect(component.goalsError()).toBe(true);
        expect(statsServiceStub.fetchStats).not.toHaveBeenCalled();
        expect(analyticsServiceStub.getGoalTimeseries).not.toHaveBeenCalled();

        component.refresh();
        TestBed.tick();
        expect(component.goalsError()).toBe(false);
        expect(statsServiceStub.fetchStats).toHaveBeenCalledTimes(1);
    });

    it('clears the report and cancels the in-flight cohort after deleting the last goal', () => {
        analyticsServiceStub.getGoals.mockReturnValueOnce(of([goal]));
        const conversion = { time: '2026-07-01', conversions: 2 };
        analyticsServiceStub.getGoalTimeseries.mockReturnValue(of([conversion]));
        statsServiceStub.fetchStats.mockReturnValueOnce(of({ ...stats, unique_sessions: 2 }));
        const component = createReporting();
        TestBed.tick();
        expect(component.kpis().map((kpi) => kpi.value)).toEqual([2, 2, 50]);

        const staleCohort = new Subject<SiteStats>();
        statsServiceStub.fetchStats.mockReturnValueOnce(staleCohort);
        component.refresh();
        expect(staleCohort.observed).toBe(true);
        const baseline = new Subject<SiteStats>();
        statsServiceStub.fetchStats.mockReturnValueOnce(baseline);
        component.deleteGoal(goal);
        TestBed.tick();
        const staleSubscribed = staleCohort.observed;
        staleCohort.next(stats);
        staleCohort.complete();

        expect(analyticsServiceStub.deleteGoal).toHaveBeenCalledWith('site-1', goal.id);
        expect(staleSubscribed).toBe(false);
        expect(component.kpis().map((kpi) => kpi.value)).toEqual([0, 0, 0]);
        expect(component.goalSeries()).toEqual([]);
        expect(component.comparisonGoalSeries()).toEqual([]);
        expect(statsServiceStub.fetchStats).toHaveBeenCalledTimes(5);
        expect(analyticsServiceStub.getGoalTimeseries).toHaveBeenCalledTimes(4);

        baseline.next({ ...stats, top_pages: [{ name: '/docs', value: 6 }] });
        baseline.complete();
        expect(component.pathSuggestions()).toEqual(['/docs']);
    });

    it('loads the cohort and both series after creating the first goal', () => {
        const component = createReporting();
        TestBed.tick();
        statsServiceStub.fetchStats.mockClear();
        analyticsServiceStub.getGoalTimeseries.mockClear();

        analyticsServiceStub.getGoals.mockReturnValueOnce(of([goal]));
        analyticsServiceStub.getGoalTimeseries.mockReturnValueOnce(of([{ time: '2026-07-01', conversions: 2 }]));
        statsServiceStub.fetchStats.mockReturnValueOnce(of({ ...stats, unique_sessions: 2 }));
        component.onDefinitionsChanged();
        TestBed.tick();

        expect(statsServiceStub.fetchStats).toHaveBeenCalledTimes(2);
        expect(statsServiceStub.fetchStats).toHaveBeenNthCalledWith(1, 'site-1', expect.any(String), expect.any(String), [], [goal.id]);
        expect(analyticsServiceStub.getGoalTimeseries).toHaveBeenCalledTimes(2);
        expect(component.kpis().map((kpi) => kpi.value)).toEqual([2, 2, 50]);
    });
});
