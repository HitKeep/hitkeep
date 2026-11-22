import { afterEach, describe, expect, it, vi } from 'vitest';

import { cleanup, init, TRACKER_VERSION } from './package';
import { beaconPayload, packageHarness } from '../../testing/tracker';
import { bootstrapWebVitals } from './vitals';

const metrics = vi.hoisted(() => ({
    onCLS: vi.fn(),
    onFCP: vi.fn(),
    onINP: vi.fn(),
    onLCP: vi.fn(),
    onTTFB: vi.fn()
}));

vi.mock('web-vitals', () => metrics);

describe('web vitals tracker bundle', () => {
    afterEach(cleanup);

    it.each(['before', 'after'])('reports to the current SDK when the split bundle loads %s cleanup', async (timing) => {
        const { win, sendBeacon } = packageHarness();
        const config = { host: 'https://stats.example.com/first', webVitals: true };
        vi.mocked(win.crypto.randomUUID).mockImplementation(() => crypto.randomUUID());
        vi.clearAllMocks();

        init(config, win);
        if (timing === 'before') bootstrapWebVitals(win);
        const firstPage = await beaconPayload(sendBeacon, 0);
        cleanup();
        expect(win.hk?._webVitals).toBeUndefined();
        if (timing === 'after') bootstrapWebVitals(win);

        for (const register of Object.values(metrics)) expect(register).toHaveBeenCalledTimes(1);
        const report = metrics.onLCP.mock.calls[0]![0];
        const metric = { name: 'LCP', value: 1842.3, id: 'v5-1234567890-1234567890123', rating: 'good', navigationType: 'navigate' };
        report(metric);
        expect(sendBeacon).toHaveBeenCalledTimes(1);

        init({ ...config, webVitals: false, autoCapturePageviews: false }, win);
        report(metric);
        expect(sendBeacon).toHaveBeenCalledTimes(1);
        cleanup();

        win.sessionStorage.setItem('hk_session', '');
        win.history.pushState({}, '', '/checkout');
        init({ ...config, host: 'https://stats.example.com/second' }, win);
        const currentPage = await beaconPayload(sendBeacon, 1);
        expect(currentPage['session_id']).not.toBe(firstPage['session_id']);
        expect(currentPage['page_id']).not.toBe(firstPage['page_id']);
        expect(win.document.head.appendChild).toHaveBeenCalledTimes(1);

        report(metric);
        expect(sendBeacon.mock.calls.map(([url]) => url)).toEqual(['https://stats.example.com/first/ingest', 'https://stats.example.com/second/ingest', 'https://stats.example.com/second/ingest/web-vitals']);
        expect(await beaconPayload(sendBeacon, 2)).toEqual({
            n: 'LCP',
            v: metric.value,
            p: '/checkout',
            nt: 'navigate',
            mid: metric.id,
            sid: currentPage['session_id'],
            pid: currentPage['page_id'],
            tsrc: '@hitkeep/tracker',
            tv: TRACKER_VERSION,
            ua: win.navigator.userAgent
        });
    });
});
