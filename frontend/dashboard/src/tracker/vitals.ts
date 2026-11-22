import { onCLS, onFCP, onINP, onLCP, onTTFB, type Metric } from 'web-vitals';

import type { HitKeepWindow, WebVitalsTrackerContext } from './core';

function navigationType(metric: Metric): string | undefined {
    const value = metric.navigationType;
    return typeof value === 'string' && value.length > 0 ? value : undefined;
}

function emitMetric(context: WebVitalsTrackerContext, metric: Metric): void {
    context.emit({
        n: metric.name,
        v: metric.value,
        p: context.getPath(),
        nt: navigationType(metric),
        mid: metric.id,
        sid: context.sessionId,
        pid: context.pageId(),
        tsrc: context.trackerSource,
        tv: context.trackerVersion,
        ua: context.userAgent
    });
}

export function bootstrapWebVitals(win: HitKeepWindow = window): void {
    // The requested script can finish loading between SDK mounts.
    if (!win.hk?._webVitalsLoaded) return;

    const report = (metric: Metric) => {
        const context = win.hk?._webVitals;
        if (context) emitMetric(context, metric);
    };
    onCLS(report);
    onFCP(report);
    onINP(report);
    onLCP(report);
    onTTFB(report);
}

(() => {
    try {
        bootstrapWebVitals();
    } catch {
        // Keep the optional bundle silent. The main tracker must remain best-effort.
    }
})();
