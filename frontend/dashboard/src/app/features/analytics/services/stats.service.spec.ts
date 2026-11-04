import { TestBed } from '@angular/core/testing';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideHttpClient } from '@angular/common/http';
import { StatsService } from '@features/analytics/services/stats.service';
import { resolveDateRange } from '@components/range-toolbar/range-options';

describe('StatsService', () => {
    let service: StatsService;
    let httpMock: HttpTestingController;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [StatsService, provideHttpClient(), provideHttpClientTesting()]
        });
        service = TestBed.inject(StatsService);
        httpMock = TestBed.inject(HttpTestingController);
    });

    afterEach(() => {
        httpMock.verify();
    });

    it('should be created', () => {
        expect(service).toBeTruthy();
    });

    it('requests an elapsed DST window and an equal comparison ending one millisecond before it', () => {
        const range = resolveDateRange({ value: '30m' }, null, new Date('2026-03-29T01:10:00.123Z'))!;
        service.fetchStats('site', range.from, range.to).subscribe();

        const req = httpMock.expectOne((request) => request.method === 'GET' && request.url === '/api/sites/site/stats');
        expect(req.request.params.get('from')).toBe('2026-03-29T00:40:00.123Z');
        expect(req.request.params.get('to')).toBe('2026-03-29T01:10:00.123Z');
        expect(req.request.params.get('compare_from')).toBe('2026-03-29T00:10:00.122Z');
        expect(req.request.params.get('compare_to')).toBe('2026-03-29T00:40:00.122Z');
        req.flush({});
    });

    it('requests lightweight overview stats for all accessible sites', () => {
        service.fetchSitesOverviewStats('2026-07-01T00:00:00.000Z', '2026-07-02T00:00:00.000Z').subscribe((response) => {
            expect(response.sites).toEqual([]);
        });

        const req = httpMock.expectOne((request) => request.method === 'GET' && request.url === '/api/sites/overview');
        expect(req.request.params.get('from')).toBe('2026-07-01T00:00:00.000Z');
        expect(req.request.params.get('to')).toBe('2026-07-02T00:00:00.000Z');
        expect(req.request.params.has('compare_from')).toBe(false);
        req.flush({ sites: [] });
    });
});
