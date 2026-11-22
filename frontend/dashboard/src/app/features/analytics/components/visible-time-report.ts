import { HttpErrorResponse, httpResource } from '@angular/common/http';
import { ChangeDetectionStrategy, Component, computed, input, signal } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { injectActiveLang } from '@core/i18n/active-lang';
import { localeForLanguage } from '@core/i18n/duration-format';
import type { VisibleTimePage } from '@models/analytics.types';

interface VisibleTimeScope {
    siteId: string;
    from: string;
    to: string;
    filters: { type: string; value: string }[];
    subject: { kind: 'goal' | 'funnel'; id: string } | null;
}

@Component({
    selector: 'app-visible-time-report',
    imports: [ButtonModule, TranslocoPipe],
    templateUrl: './visible-time-report.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class VisibleTimeReport {
    readonly scope = input.required<VisibleTimeScope>();
    readonly refreshKey = input(0);
    protected readonly open = signal(false);

    private readonly activeLang = injectActiveLang();
    private readonly locale = computed(() => localeForLanguage(this.activeLang()));
    protected readonly seconds = computed(() => new Intl.NumberFormat(this.locale(), { style: 'unit', unit: 'second', maximumFractionDigits: 3 }));
    protected readonly counts = computed(() => new Intl.NumberFormat(this.locale()));

    protected readonly report = httpResource<VisibleTimePage[]>(() => {
        if (!this.open()) return undefined;
        this.refreshKey();
        const { siteId, from, to, filters, subject } = this.scope();
        return {
            url: `/api/sites/${siteId}/visible-time`,
            params: {
                from,
                to,
                filter: filters.map(({ type, value }) => `${type}:${value}`),
                ...(subject ? { [`${subject.kind}_id`]: subject.id } : {})
            }
        };
    });

    protected readonly forbidden = computed(() => {
        const error = this.report.error();
        return error instanceof HttpErrorResponse && (error.status === 401 || error.status === 403);
    });
}
