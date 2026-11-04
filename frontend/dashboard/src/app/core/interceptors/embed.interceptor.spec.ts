import { HttpClient, provideHttpClient, withInterceptors } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { EmbedService } from '@services/embed.service';
import { authInterceptor } from './auth.interceptor';
import { embedInterceptor } from './embed.interceptor';
import { basePathInterceptor } from './base-path.interceptor';
import { vi } from 'vitest';

describe('embedded analytics request boundary', () => {
    const credential = signal({ token: 'hk_embed_secret', siteId: 'site-a', parentOrigin: 'https://wordpress.example' });
    const embed = { isEmbedMode: () => true, credential, fail: vi.fn(), requestRefresh: vi.fn() };
    let http: HttpClient;
    let requests: HttpTestingController;

    beforeEach(() => {
        vi.clearAllMocks();
        TestBed.configureTestingModule({ providers: [provideHttpClient(withInterceptors([embedInterceptor, authInterceptor, basePathInterceptor])), provideHttpClientTesting(), { provide: EmbedService, useValue: embed }] });
        http = TestBed.inject(HttpClient);
        requests = TestBed.inject(HttpTestingController);
    });

    afterEach(() => requests.verify());

    it('sends the dedicated bearer and parent binding only on single-site aggregate reads, without cookies', () => {
        http.get('/api/sites/site-a/stats').subscribe();
        const request = requests.expectOne('/api/sites/site-a/stats');
        expect(request.request.headers.get('Authorization')).toBe('Bearer hk_embed_secret');
        expect(request.request.headers.get('X-HitKeep-Embed-Parent')).toBe('https://wordpress.example');
        expect(request.request.withCredentials).toBe(false);
        expect(request.request.credentials).toBe('omit');
        request.flush({});
    });

    it('keeps same-origin subdirectory deployments within their API base path', () => {
        const base = document.querySelector('base');
        const previous = base?.getAttribute('href');
        const element = base ?? document.head.appendChild(document.createElement('base'));
        element.setAttribute('href', '/hitkeep/');
        try {
            http.get(`${document.location.origin}/hitkeep/api/sites/site-a/stats`).subscribe();
            const request = requests.expectOne(`${document.location.origin}/hitkeep/api/sites/site-a/stats`);
            expect(request.request.headers.get('Authorization')).toBe('Bearer hk_embed_secret');
            request.flush({});
        } finally {
            if (!base) element.remove();
            else if (previous !== null) base.setAttribute('href', previous!);
            else base.removeAttribute('href');
        }
    });

    it('blocks another site, writes, account APIs, raw hits and exports before network access', () => {
        for (const path of ['/api/sites/site-b/stats', '/api/user/permissions', '/api/sites/site-a/hits', '/api/sites/site-a/hits/export', '/api/sites/site-a/goals/../share', '/api/sites/site-a/stats/']) {
            const error = vi.fn();
            http.get(path).subscribe({ error });
            expect(error).toHaveBeenCalledWith(expect.objectContaining({ status: 403 }));
            requests.expectNone(path);
        }
        const error = vi.fn();
        http.post('/api/sites/site-a/goals', {}).subscribe({ error });
        expect(error).toHaveBeenCalledWith(expect.objectContaining({ status: 403 }));
    });

    it('never exposes the credential to external URLs or translation assets', () => {
        for (const path of ['https://evil.example/api/sites/site-a/stats', '//evil.example/api/sites/site-a/stats', '/i18n/en.json']) {
            http.get(path).subscribe();
            const request = requests.expectOne(path);
            expect(request.request.headers.has('Authorization')).toBe(false);
            expect(request.request.headers.has('X-HitKeep-Embed-Parent')).toBe(false);
            expect(request.request.withCredentials).toBe(false);
            expect(request.request.credentials).toBe('omit');
            request.flush({});
        }
    });

    it('refreshes through the parent after an expired credential without a dashboard login redirect', () => {
        const error = vi.fn();
        http.get('/api/sites/site-a/stats').subscribe({ error });
        requests.expectOne('/api/sites/site-a/stats').flush(null, { status: 401, statusText: 'Expired' });
        expect(embed.fail).toHaveBeenCalledOnce();
        expect(embed.requestRefresh).toHaveBeenCalledOnce();
        expect(error).toHaveBeenCalledWith(expect.objectContaining({ status: 401 }));
    });
});
