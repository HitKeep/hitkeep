import { inject, Service, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Router } from '@angular/router';
import { finalize, timeout } from 'rxjs';
import { ApplicationErrorNavigationService, ROUTE_CRITICAL_REQUEST_TIMEOUT_MS } from '@services/application-error-navigation.service';

import { OAuthConsent, OAuthGrant } from '@models/oauth.types';

@Service()
export class OAuthService {
    private readonly http = inject(HttpClient);
    private readonly router = inject(Router);
    private readonly applicationErrors = inject(ApplicationErrorNavigationService);
    readonly resuming = signal(false);

    continueAfterAuthentication(fallback: string): void {
        if (this.resuming()) return;
        const destination = this.router.parseUrl(fallback);
        const path = fallback.split(/[?#]/, 1)[0];
        if (path === '/signup/verified' || ((path === '/admin/team' || path === '/admin/team/overview') && destination.queryParams['purchase'] === 'review')) {
            void this.router.navigateByUrl(fallback);
            return;
        }
        this.resuming.set(true);
        this.continuation()
            .pipe(finalize(() => this.resuming.set(false)))
            .subscribe({
                next: ({ pending }) => void this.router.navigateByUrl(pending ? '/oauth/consent' : fallback),
                error: (error: unknown) => {
                    const command = this.applicationErrors.fromHttp(error, 'dashboard-bootstrap', '/oauth/consent');
                    void this.router.navigateByUrl(command.redirectTo, command.navigationBehaviorOptions);
                }
            });
    }

    consent() {
        return this.http.get<OAuthConsent>('/api/oauth/consent').pipe(timeout({ first: ROUTE_CRITICAL_REQUEST_TIMEOUT_MS }));
    }

    continuation() {
        return this.http.get<{ pending: boolean }>('/api/oauth/continuation').pipe(timeout({ first: ROUTE_CRITICAL_REQUEST_TIMEOUT_MS }));
    }

    grants() {
        return this.http.get<OAuthGrant[]>('/api/user/oauth-grants').pipe(timeout({ first: ROUTE_CRITICAL_REQUEST_TIMEOUT_MS }));
    }

    revoke(id: string) {
        return this.http.delete<void>(`/api/user/oauth-grants/${encodeURIComponent(id)}`).pipe(timeout({ first: ROUTE_CRITICAL_REQUEST_TIMEOUT_MS }));
    }
}
