import { DOCUMENT } from '@angular/common';
import { HttpErrorResponse, HttpInterceptorFn } from '@angular/common/http';
import { inject } from '@angular/core';
import { catchError, throwError } from 'rxjs';
import { EmbedService } from '@services/embed.service';
import { browserBasePath, prefixBrowserBasePath } from './base-path.interceptor';

const ANALYTICS_PATH =
    /^(?:stats|ai-activity|goals(?:\/timeseries)?|funnels(?:\/timeseries|\/[a-f0-9-]+\/stats)?|events\/(?:names|properties|breakdown|timeseries|audience)|ecommerce(?:\/(?:timeseries|products|sources))?|web-vitals\/(?:summary|timeseries|pages|breakdown))$/;

export const embedInterceptor: HttpInterceptorFn = (request, next) => {
    const embed = inject(EmbedService);
    if (!embed.isEmbedMode()) return next(request);
    const document = inject(DOCUMENT);
    const origin = document.location.origin;
    const url = new URL(prefixBrowserBasePath(request.url, browserBasePath(document)), origin);
    const apiPath = `${browserBasePath(document)}api/`;
    if (url.origin !== origin || !url.pathname.startsWith(apiPath)) return next(request.clone({ withCredentials: false, credentials: 'omit' }));
    const credential = embed.credential();
    const path = url.pathname.slice(apiPath.length);
    const sitePrefix = credential ? `sites/${encodeURIComponent(credential.siteId)}/` : '';
    const allowed =
        credential && request.method === 'GET' && !url.username && !url.password && (path === `embed/sites/${encodeURIComponent(credential.siteId)}/site` || (path.startsWith(sitePrefix) && ANALYTICS_PATH.test(path.slice(sitePrefix.length))));
    if (!allowed) return throwError(() => new HttpErrorResponse({ status: 403, statusText: 'Embedded analytics access denied' }));
    return next(request.clone({ withCredentials: false, credentials: 'omit', setHeaders: { Authorization: `Bearer ${credential.token}`, 'X-HitKeep-Embed-Parent': credential.parentOrigin } })).pipe(
        catchError((error: unknown) => {
            if (error instanceof HttpErrorResponse && (error.status === 401 || error.status === 403) && embed.credential()?.token === credential.token) {
                embed.fail();
                embed.requestRefresh();
            }
            return throwError(() => error);
        })
    );
};
