import { DestroyRef, inject, signal, Service } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { defer, type Observable, type Subscription, throwIfEmpty } from 'rxjs';
import { type ReportWindow, reportWindowBounds } from '@components/range-toolbar/range-options';
import { isTransientReportError } from '@core/analytics/transient-report-error';
import { Hit, PaginatedHits } from '@models/analytics.types';

@Service()
export class HitService {
    private http = inject(HttpClient);

    readonly hits = signal<Hit[]>([]);
    readonly total = signal<number>(0);
    readonly isLoading = signal<boolean>(false);
    readonly hasError = signal<boolean>(false);

    private currentPage: Observable<PaginatedHits> | null = null;
    private activeRequest: Subscription | null = null;
    private refreshPending = false;

    constructor() {
        inject(DestroyRef).onDestroy(() => this.reset());
    }

    loadHits(
        siteId: string,
        window: ReportWindow,
        page = 1,
        pageSize = 10,
        sortField?: string,
        sortOrder?: string,
        query?: string,
        filters: { type: string; value: string }[] = [],
        goalIds: string[] = [],
        funnelIds: string[] = [],
        shareToken?: string | null
    ) {
        this.reset();
        this.isLoading.set(true);

        let params = new HttpParams().set('limit', pageSize).set('offset', (page - 1) * pageSize);

        if (sortField) params = params.set('sort', sortField);
        if (sortOrder) params = params.set('order', sortOrder);
        if (query) params = params.set('q', query);
        for (const filter of filters) {
            params = params.append('filter', `${filter.type}:${filter.value}`);
        }
        for (const id of goalIds) params = params.append('goal_id', id);
        for (const id of funnelIds) params = params.append('funnel_id', id);

        const endpoint = shareToken ? `/api/share/${encodeURIComponent(shareToken)}/sites/${siteId}/hits` : `/api/sites/${siteId}/hits`;
        // Session expiry can complete the HTTP stream without a response during the redirect.
        this.currentPage = defer(() => {
            const { from, to } = reportWindowBounds(window);
            return this.http.get<PaginatedHits>(endpoint, { params: params.set('from', from).set('to', to) });
        }).pipe(throwIfEmpty());
        this.fetchPage();
    }

    refresh() {
        if (this.activeRequest && !this.activeRequest.closed) {
            this.refreshPending = true;
            return;
        }
        this.fetchPage();
    }

    private fetchPage() {
        if (!this.currentPage) return;
        this.refreshPending = false;
        this.activeRequest = this.currentPage.subscribe({
            next: (res) => {
                this.hits.set(res.data);
                this.total.set(res.total);
                this.isLoading.set(false);
                this.hasError.set(false);
            },
            error: (error) => {
                this.hits.set([]);
                this.total.set(0);
                this.isLoading.set(false);
                this.hasError.set(true);
                if (!isTransientReportError(error)) this.currentPage = null;
                if (this.refreshPending) this.fetchPage();
            },
            complete: () => {
                if (this.refreshPending) this.fetchPage();
            }
        });
    }

    reset() {
        this.activeRequest?.unsubscribe();
        this.activeRequest = null;
        this.currentPage = null;
        this.refreshPending = false;
        this.hits.set([]);
        this.total.set(0);
        this.isLoading.set(false);
        this.hasError.set(false);
    }
}
