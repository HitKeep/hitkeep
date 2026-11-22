import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { VisibleTimeReport } from './visible-time-report';

describe('VisibleTimeReport', () => {
    let fixture: ComponentFixture<VisibleTimeReport>;
    let http: HttpTestingController;
    const scope = {
        siteId: 'site-1',
        from: '2026-11-19T00:00:00Z',
        to: '2026-11-20T23:59:59Z',
        filters: [{ type: 'country', value: 'US' }],
        subject: { kind: 'goal', id: 'goal-1' }
    };

    function setOpen(open: boolean) {
        const details = fixture.nativeElement.querySelector('details') as HTMLDetailsElement;
        details.open = open;
        details.dispatchEvent(new Event('toggle'));
        fixture.detectChanges();
    }

    beforeEach(() => {
        TestBed.configureTestingModule({
            imports: [
                VisibleTimeReport,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            dashboard: { visibleTime: { missing: 'No sample', empty: 'No matching pageviews', unavailable: 'Report unavailable', forbidden: 'Access denied' } },
                            common: { loadingSiteData: 'Loading', retry: 'Try again' }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ],
            providers: [provideHttpClient(), provideHttpClientTesting()]
        });
        http = TestBed.inject(HttpTestingController);
        fixture = TestBed.createComponent(VisibleTimeReport);
        fixture.componentRef.setInput('scope', scope);
        fixture.detectChanges();
    });

    afterEach(() => http.verify());

    it('waits to open, uses the selected scope, and displays zero and missing separately', async () => {
        fixture.componentRef.setInput('refreshKey', 1);
        fixture.detectChanges();
        http.expectNone(() => true);
        setOpen(true);

        const request = http.expectOne((req) => req.url === '/api/sites/site-1/visible-time');
        expect(request.request.params.get('from')).toBe(scope.from);
        expect(request.request.params.get('to')).toBe(scope.to);
        expect(request.request.params.getAll('filter')).toEqual(['country:US']);
        expect(request.request.params.get('goal_id')).toBe('goal-1');
        expect(fixture.nativeElement.textContent).toContain('Loading');

        request.flush([
            { path: '/pricing', avg_visible_ms: 0, measured_views: 1, tracked_views: 3 },
            { path: '/help', avg_visible_ms: null, measured_views: 0, tracked_views: 2 }
        ]);
        await fixture.whenStable();
        const rows: NodeListOf<HTMLTableRowElement> = fixture.nativeElement.querySelectorAll('tbody tr');
        expect(rows[0].textContent).toMatch(/\/pricing.*0 sec.*1 \/ 3/s);
        expect(rows[1].textContent).toMatch(/\/help.*No sample.*0 \/ 2/s);

        fixture.componentRef.setInput('refreshKey', 2);
        fixture.detectChanges();
        const refresh = http.expectOne((req) => req.url === '/api/sites/site-1/visible-time');
        expect(fixture.nativeElement.querySelector('table')).toBeNull();
        refresh.flush([]);
        await fixture.whenStable();
        expect(fixture.nativeElement.textContent).toContain('No matching pageviews');
    });

    it('cancels obsolete requests and retries a failed report', async () => {
        setOpen(true);
        const old = http.expectOne((req) => req.url === '/api/sites/site-1/visible-time');

        fixture.componentRef.setInput('scope', { ...scope, siteId: 'site-2', from: '2026-11-18T00:00:00Z', subject: null });
        fixture.detectChanges();
        expect(old.cancelled).toBe(true);
        const latest = http.expectOne((req) => req.url === '/api/sites/site-2/visible-time');
        expect(latest.request.params.get('from')).toBe('2026-11-18T00:00:00Z');
        expect(latest.request.params.has('goal_id')).toBe(false);

        latest.flush('', { status: 500, statusText: 'Server error' });
        await fixture.whenStable();
        expect(fixture.nativeElement.textContent).toContain('Report unavailable');
        fixture.nativeElement.querySelector('button').click();
        fixture.detectChanges();
        const retry = http.expectOne((req) => req.url === '/api/sites/site-2/visible-time');
        retry.flush('', { status: 403, statusText: 'Forbidden' });
        await fixture.whenStable();
        expect(fixture.nativeElement.textContent).toContain('Access denied');
        expect(fixture.nativeElement.querySelector('table')).toBeNull();

        setOpen(false);
        fixture.componentRef.setInput('refreshKey', 1);
        fixture.detectChanges();
        http.expectNone(() => true);
    });
});
