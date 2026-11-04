import { DOCUMENT } from '@angular/common';
import { provideHttpClient } from '@angular/common/http';
import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';

import { SiteService } from '@features/sites/services/site.service';
import { ShareService } from '@services/share.service';
import { RealtimeService } from '@services/realtime.service';
import { EmbedService } from '@services/embed.service';

class MockEventSource {
    static instances: MockEventSource[] = [];

    onopen: ((event: Event) => void) | null = null;
    onerror: ((event: Event) => void) | null = null;
    closed = false;
    private readonly listeners = new Map<string, EventListenerOrEventListenerObject[]>();

    constructor(
        readonly url: string,
        readonly options?: EventSourceInit
    ) {
        MockEventSource.instances.push(this);
    }

    addEventListener(type: string, listener: EventListenerOrEventListenerObject): void {
        const listeners = this.listeners.get(type) ?? [];
        listeners.push(listener);
        this.listeners.set(type, listeners);
    }

    emit(type: string, data: unknown): void {
        const event = new MessageEvent(type, { data: JSON.stringify(data) });
        for (const listener of this.listeners.get(type) ?? []) {
            if (typeof listener === 'function') {
                listener(event);
            } else {
                listener.handleEvent(event);
            }
        }
    }

    close(): void {
        this.closed = true;
    }
}

const flushEffects = (): void => {
    const testBed = TestBed as unknown as { flushEffects?: () => void; tick?: () => void };
    testBed.flushEffects?.();
    testBed.tick?.();
};

describe('RealtimeService', () => {
    beforeEach(() => {
        MockEventSource.instances = [];
        vi.stubGlobal('EventSource', MockEventSource);
        TestBed.configureTestingModule({
            providers: [provideHttpClient(), { provide: DOCUMENT, useValue: document }]
        });
    });

    afterEach(() => {
        TestBed.resetTestingModule();
        vi.unstubAllGlobals();
    });

    it('keeps one EventSource for the active site and replaces it when the site changes', () => {
        const service = TestBed.inject(RealtimeService);
        const siteService = TestBed.inject(SiteService);

        siteService.activeSite.set({ id: 'site-1', user_id: 'user-1', domain: 'example.com', created_at: '2026-01-01T00:00:00Z' });
        flushEffects();

        expect(MockEventSource.instances.length).toBe(1);
        expect(MockEventSource.instances[0].url).toContain('/api/sites/site-1/realtime');
        expect(MockEventSource.instances[0].options).toEqual({ withCredentials: true });

        siteService.activeSite.set({ id: 'site-1', user_id: 'user-1', domain: 'example.com', created_at: '2026-01-01T00:00:00Z' });
        flushEffects();
        expect(MockEventSource.instances.length).toBe(1);

        siteService.activeSite.set({ id: 'site-2', user_id: 'user-1', domain: 'example.org', created_at: '2026-01-01T00:00:00Z' });
        flushEffects();

        expect(MockEventSource.instances.length).toBe(2);
        expect(MockEventSource.instances[0].closed).toBe(true);
        expect(MockEventSource.instances[1].url).toContain('/api/sites/site-2/realtime');
        expect(service.activeSiteId()).toBe('site-2');
    });

    it('uses the share realtime endpoint when a share token is active', () => {
        TestBed.inject(RealtimeService);
        const shareService = TestBed.inject(ShareService);

        shareService.token.set('share-token');
        shareService.site.set({ id: 'site-1', user_id: 'user-1', domain: 'example.com', created_at: '2026-01-01T00:00:00Z' });
        flushEffects();

        expect(MockEventSource.instances.length).toBe(1);
        expect(MockEventSource.instances[0].url).toContain('/api/share/share-token/sites/site-1/realtime');
    });

    it('uses the existing polling fallback without opening a cookie-authenticated stream in an embed', () => {
        TestBed.overrideProvider(EmbedService, { useValue: { isEmbedMode: () => true } });
        const service = TestBed.inject(RealtimeService);
        TestBed.inject(SiteService).activeSite.set({ id: 'site-1', user_id: '', domain: 'example.com', created_at: '' });
        flushEffects();
        expect(MockEventSource.instances).toHaveLength(0);
        expect(service.status()).toBe('idle');
    });

    it('emits parsed analytics events and tracks stream status', () => {
        const service = TestBed.inject(RealtimeService);
        const siteService = TestBed.inject(SiteService);
        const received: unknown[] = [];
        service.events$.subscribe((event) => received.push(event));

        siteService.activeSite.set({ id: 'site-1', user_id: 'user-1', domain: 'example.com', created_at: '2026-01-01T00:00:00Z' });
        flushEffects();
        MockEventSource.instances[0].onopen?.(new Event('open'));

        expect(service.status()).toBe('open');

        MockEventSource.instances[0].emit('analytics.changed', {
            site_id: 'site-1',
            kinds: ['hits'],
            changed_at: '2026-06-01T12:00:00Z',
            bucket_start: '2026-06-01T12:00:00Z',
            counts: { hits: 1 }
        });

        expect(received).toEqual([
            {
                type: 'analytics.changed',
                site_id: 'site-1',
                kinds: ['hits'],
                changed_at: '2026-06-01T12:00:00Z',
                bucket_start: '2026-06-01T12:00:00Z',
                counts: { hits: 1 }
            }
        ]);
    });
});
