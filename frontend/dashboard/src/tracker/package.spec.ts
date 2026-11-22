import { vi } from 'vitest';

import { blockTrackingForMe, cleanup, enableTrackingForMe, init, isTrackingEnabled, track, trackPageview, trackPurchase, TRACKER_VERSION } from './package';

import { bootstrapTracker } from './core';
import { beaconPayload, packageHarness, trackerHarness } from '../../testing/tracker';

describe('@hitkeep/tracker package entry', () => {
    afterEach(() => {
        cleanup();
        vi.unstubAllGlobals();
    });

    it('requires a host', () => {
        expect(() => init({ host: '' })).toThrowError(/host/);
        expect(() => init({ host: '   ' })).toThrowError(/host/);
    });

    it('initializes from a host and sends the initial pageview to derived endpoints', async () => {
        const harness = packageHarness();

        init({ host: 'https://stats.example.com/' }, harness.win);

        expect(harness.sendBeacon).toHaveBeenCalledTimes(1);
        expect(harness.sendBeacon.mock.calls[0]?.[0]).toBe('https://stats.example.com/ingest');
        const payload = await beaconPayload(harness.sendBeacon, 0);
        expect(payload['tsrc']).toBe('@hitkeep/tracker');
        expect(payload['tv']).toBe(TRACKER_VERSION);
        expect(payload).not.toHaveProperty('pc');
    });

    it('supports path-prefixed hosts', () => {
        const harness = packageHarness();

        const handle = init({ host: 'https://www.example.net/hitkeep' }, harness.win);
        handle.track('signup_clicked');

        expect(harness.sendBeacon.mock.calls.map(([url]) => url)).toEqual(['https://www.example.net/hitkeep/ingest', 'https://www.example.net/hitkeep/ingest/event']);
    });

    it('measures manual pageviews through a prefix-relative keepalive POST when beacons are disabled', () => {
        const harness = packageHarness();
        const fetch = vi.fn<(url: string, init: RequestInit) => Promise<Response>>().mockResolvedValue(new Response(null, { status: 202 }));
        vi.stubGlobal('fetch', fetch);
        const handle = init({ host: 'https://www.example.net/hitkeep', autoCapturePageviews: false, visibleTime: true, useBeacon: false }, harness.win);
        const hide = () => {
            for (const listener of harness.windowListeners['pagehide'] ?? []) {
                listener(new PageTransitionEvent('pagehide'));
            }
        };
        expect(fetch).not.toHaveBeenCalled();
        handle.trackPageview();
        hide();

        expect(fetch).toHaveBeenCalledTimes(2);
        const { page_id: pid } = JSON.parse(fetch.mock.calls[0]![1].body as string);
        const [url, request] = fetch.mock.calls[1]!;
        expect(url).toBe('https://www.example.net/hitkeep/ingest/visible-time');
        expect(request).toMatchObject({ method: 'POST', keepalive: true, credentials: 'omit' });
        expect(request.headers).toBeUndefined();
        expect(JSON.parse(request.body as string)).toEqual({ pid, ms: 0 });
    });

    it('tracks marked interactions with automatic events disabled and removes their listeners on cleanup', async () => {
        const harness = packageHarness();
        const handle = init(
            {
                host: 'https://stats.example.com',
                autoCapturePageviews: false,
                declarativeEvents: true,
                outboundLinks: false,
                fileDownloads: false,
                formSubmissions: false
            },
            harness.win
        );

        const button = document.createElement('button');
        button.setAttribute('data-hk-event', 'named_click');
        const form = document.createElement('form');
        form.setAttribute('data-hk-event', 'named_submit');
        for (const [event, target] of [
            [new MouseEvent('click'), button],
            [new SubmitEvent('submit'), form]
        ] as const) {
            for (const listener of harness.documentListeners[event.type] ?? []) {
                target.addEventListener(event.type, listener, { once: true });
            }
            target.dispatchEvent(event);
        }

        expect(harness.sendBeacon).toHaveBeenCalledTimes(2);
        expect(await beaconPayload(harness.sendBeacon, 0)).toMatchObject({ n: 'named_click', p: {} });
        expect(await beaconPayload(harness.sendBeacon, 1)).toMatchObject({ n: 'named_submit', p: {} });

        handle.cleanup();
        expect(harness.documentListeners['click']).toHaveLength(0);
        expect(harness.documentListeners['auxclick']).toHaveLength(0);
        expect(harness.documentListeners['submit']).toHaveLength(0);
    });

    it('binds window.hk.event by default and skips it when disabled', () => {
        const bound = packageHarness();
        init({ host: 'https://stats.example.com' }, bound.win);
        expect(typeof bound.win.hk?.event).toBe('function');
        cleanup();

        const unbound = packageHarness();
        init({ host: 'https://stats.example.com', bindToWindow: false }, unbound.win);
        expect(unbound.win.hk?.event).toBeUndefined();
    });

    it('queues events tracked before init and flushes them once initialized', async () => {
        const harness = packageHarness();

        track('early_event', { step: 1 });
        trackPurchase({ transaction_id: 'tx-1', value: 49.9, currency: 'EUR' });
        expect(harness.sendBeacon).not.toHaveBeenCalled();

        init({ host: 'https://stats.example.com', autoCapturePageviews: false }, harness.win);

        expect(harness.sendBeacon).toHaveBeenCalledTimes(2);
        const first = await beaconPayload(harness.sendBeacon, 0);
        const second = await beaconPayload(harness.sendBeacon, 1);
        expect(first['n']).toBe('early_event');
        expect(second['n']).toBe('purchase');
        expect((second['p'] as Record<string, unknown>)['transaction_id']).toBe('tx-1');
    });

    it('replays pre-init, ecommerce, and live events only after prerender activation', () => {
        const fetchMock = vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>().mockResolvedValue(new Response(null, { status: 204 }));
        vi.stubGlobal('fetch', fetchMock);
        const harness = packageHarness('/checkout?utm_source=speculative');
        harness.setPrerendering(true);

        track('early_event', { step: 1 });
        trackPurchase({ transaction_id: 'tx-1', value: 49.9, currency: 'EUR' });
        const handle = init({ host: 'https://stats.example.com', enablePaidSignals: true }, harness.win);
        handle.track('live_event', { step: 2 });
        harness.win.history.replaceState({}, '', '/confirmation?utm_source=activated&gclid=private');

        expect(harness.sendBeacon).not.toHaveBeenCalled();
        expect(fetchMock).not.toHaveBeenCalled();

        harness.setPrerendering(false);
        harness.documentListeners['prerenderingchange']?.[0]?.(new Event('prerenderingchange'));

        expect(fetchMock.mock.calls.map(([url]) => url)).toEqual(['https://stats.example.com/ingest', 'https://stats.example.com/ingest/event', 'https://stats.example.com/ingest/event', 'https://stats.example.com/ingest/event']);
        const [pageview, ...events] = fetchMock.mock.calls.map(([, request]) => JSON.parse(String(request?.body)) as Record<string, unknown>);
        expect(pageview).toMatchObject({ path: '/confirmation', u_src: 'activated', pc: true });
        expect(events).toMatchObject([
            { n: 'early_event', path: '/confirmation' },
            { n: 'purchase', path: '/confirmation' },
            { n: 'live_event', path: '/confirmation' }
        ]);
        expect(events.every((event) => !Object.hasOwn(event, 'pc'))).toBe(true);
    });

    it('returns the existing handle when initialized twice', () => {
        const harness = packageHarness();

        init({ host: 'https://stats.example.com' }, harness.win);
        init({ host: 'https://other.example.com' }, harness.win);

        expect(harness.sendBeacon).toHaveBeenCalledTimes(1);
        expect(harness.sendBeacon.mock.calls[0]?.[0]).toBe('https://stats.example.com/ingest');
    });

    it('is a no-op when the hk.js snippet already bootstrapped the page', () => {
        const harness = trackerHarness();
        bootstrapTracker(harness.win);
        const snippetEvent = harness.win.hk?.event;

        const handle = init({ host: 'https://stats.example.com' }, harness.win);
        handle.track('ignored');
        trackPageview();
        handle.cleanup();

        expect(harness.win.hk?.event).toBe(snippetEvent);
        expect(harness.win.hk?._bootstrapped).toBe(true);
        harness.win.hk?.event?.('from_snippet');
        expect(harness.sendBeacon.mock.calls.map(([url]) => url)).toEqual(['https://analytics.example.com/ingest', 'https://analytics.example.com/ingest/event']);
    });

    it('exposes the opt-out trio backed by localStorage', () => {
        const harness = packageHarness();

        expect(isTrackingEnabled(harness.win)).toBe(true);
        blockTrackingForMe(harness.win);
        expect(isTrackingEnabled(harness.win)).toBe(false);
        expect(harness.win.localStorage.getItem('hk_ignore')).toBe('true');

        const config = { host: 'https://stats.example.com/subpath', autoCapturePageviews: false };
        track('queued_before_blocked_init');
        const handle = init(config, harness.win);
        handle.track('ignored');
        handle.cleanup();
        expect(harness.win.hk?.event).toBeUndefined();
        expect(harness.win.hk?.getSessionId).toBeUndefined();

        init(config, harness.win).trackPageview();
        cleanup();
        cleanup();
        expect(harness.sendBeacon).not.toHaveBeenCalled();

        enableTrackingForMe(harness.win);
        expect(isTrackingEnabled(harness.win)).toBe(true);
        init(config, harness.win).track('enabled');
        expect(harness.sendBeacon.mock.calls.map(([url]) => url)).toEqual(['https://stats.example.com/subpath/ingest/event']);
    });

    it('respects capture on localhost', () => {
        const local = packageHarness();
        (local.win as { location: { hostname: string } }).location.hostname = 'localhost';

        init({ host: 'https://stats.example.com' }, local.win);
        expect(local.sendBeacon).not.toHaveBeenCalled();
        cleanup();

        const dev = packageHarness();
        (dev.win as { location: { hostname: string } }).location.hostname = 'localhost';
        init({ host: 'https://stats.example.com', captureOnLocalhost: true }, dev.win);
        expect(dev.sendBeacon).toHaveBeenCalledTimes(1);
    });

    it('supports cleanup and re-initialization', () => {
        const harness = packageHarness();

        init({ host: 'https://stats.example.com' }, harness.win);
        expect(harness.windowListeners['pagehide']?.length).toBe(1);
        cleanup();
        expect(harness.windowListeners['pagehide']?.length).toBe(0);

        init({ host: 'https://stats.example.com' }, harness.win);
        expect(harness.windowListeners['pagehide']?.length).toBe(1);
        expect(harness.sendBeacon).toHaveBeenCalledTimes(2);
    });
});
