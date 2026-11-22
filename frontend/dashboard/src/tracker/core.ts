export type EventProperties = Record<string, unknown>;

interface HitKeepEvent {
    name: string;
    properties: EventProperties;
}

type EventSender = (name: string, properties?: EventProperties) => void;

type InteractionEventSender = (name: string, properties?: EventProperties, declarative?: true) => void;

type HistoryMethod = 'pushState' | 'replaceState';

export interface TrackerOptions {
    collectDnt: boolean;
    disableBeacon: boolean;
    disableSpaTracking: boolean;
    disableOutboundTracking: boolean;
    disableDownloadTracking: boolean;
    disableFormTracking: boolean;
    enableDeclarativeEvents: boolean;
    enablePaidSignals: boolean;
    enableWebVitals: boolean;
    enableVisibleTime: boolean;
    trackerSource: string;
    trackerVersion: string;
}

export interface TrackerEndpoints {
    pageEndpoint: string;
    eventEndpoint: string;
    webVitalsEndpoint: string;
    visibleTimeEndpoint: string | null;
    /** Resolved only when Web Vitals are enabled; `null` keeps the split bundle unloaded. */
    webVitalsBundleUrl: string | null;
}

export interface TrackerRuntimeConfig extends TrackerOptions, TrackerEndpoints {
    capturePageviews: boolean;
    captureOnLocalhost: boolean;
    bindToWindow: boolean;
}

export interface TrackerHandle {
    track: EventSender;
    trackPageview: () => void;
    cleanup: () => void;
}

interface PendingRequest {
    endpoint: string;
    body: string;
    respectOptOut?: boolean;
}

interface VisibleTimeView {
    startedAt: number | null;
    elapsedMs: number;
    sentMs: number;
    checkpoints: number;
}

interface EventPayload {
    n: string;
    p: EventProperties;
    r: string | null;
    sid: string;
    path: string;
    ua: string;
    tsrc: string;
    tv: string;
}

type PendingActivationRequest = { kind: 'event'; endpoint: string; payload: EventPayload; declarative?: boolean } | { kind: 'web-vital'; endpoint: string; payload: WebVitalsPayload };

/** Registers a listener and records its matching teardown on the owning tracker. */
type ListenerRegistrar = (target: EventTarget, type: string, listener: EventListener, options?: boolean | AddEventListenerOptions) => void;

export type HitKeepWindow = Window &
    typeof globalThis & {
        hk?: {
            event?: EventSender;
            getSessionId?: () => string | undefined;
            _bootstrapped?: boolean;
            _webVitals?: WebVitalsTrackerContext;
            _webVitalsLoaded?: boolean;
        };
    };

export interface WebVitalsPayload {
    n: string;
    v: number;
    p: string;
    nt?: string;
    mid?: string;
    sid: string;
    pid: string;
    tsrc: string;
    tv: string;
    ua?: string;
}

export interface WebVitalsTrackerContext {
    emit: (payload: WebVitalsPayload) => void;
    getPath: () => string;
    sessionId: string;
    pageId: () => string;
    trackerSource: string;
    trackerVersion: string;
    userAgent: string;
}

type PrerenderDocument = Document & {
    readonly prerendering?: boolean;
};

const SESSION_KEY = 'hk_session';
const SESSION_EXPIRY = 30 * 60 * 1000;
const MAX_PENDING_REQUESTS = 10;
const MAX_PENDING_ACTIVATION_REQUESTS = 32;
const RETRY_DELAY_MS = 2000;
const DUPLICATE_PAGEVIEW_WINDOW_MS = 1500;
const MAX_VISIBLE_TIME_MS = 24 * 60 * 60 * 1000;
const OPT_OUT_KEY = 'hk_ignore';
const DOWNLOAD_EXTENSIONS = new Set([
    '7z',
    'avi',
    'csv',
    'doc',
    'docx',
    'epub',
    'gz',
    'ics',
    'jpeg',
    'jpg',
    'json',
    'key',
    'mov',
    'mp3',
    'mp4',
    'pdf',
    'png',
    'ppt',
    'pptx',
    'rar',
    'rtf',
    'svg',
    'tar',
    'tgz',
    'txt',
    'webp',
    'xls',
    'xlsx',
    'xml',
    'zip'
]);
const EXPLICIT_TRACKING_SELECTORS = '[data-hk-event], [data-hitkeep-event]';
const LINK_SELECTORS = 'a[href], area[href]';
const CLICK_CONTROL_SELECTORS = `${LINK_SELECTORS}, button, input, label, select, textarea`;
const noop = () => undefined;
const TRACKER_SOURCE = 'hk.js';

function ignoreError(error?: unknown): void {
    // Best-effort tracker operations should fail closed without noisy console output.
    void error;
}

export function readTrackerOptions(scriptEl: Element): TrackerOptions {
    const source = (scriptEl.getAttribute('data-hitkeep-source') || TRACKER_SOURCE).trim().slice(0, 64) || TRACKER_SOURCE;
    const version = (scriptEl.getAttribute('data-hitkeep-version') || '').trim().slice(0, 64);
    return {
        collectDnt: scriptEl.getAttribute('data-collect-dnt') === 'true',
        disableBeacon: scriptEl.getAttribute('data-disable-beacon') === 'true',
        disableSpaTracking: scriptEl.getAttribute('data-disable-spa-tracking') === 'true',
        disableOutboundTracking: scriptEl.getAttribute('data-disable-outbound-tracking') === 'true',
        disableDownloadTracking: scriptEl.getAttribute('data-disable-download-tracking') === 'true',
        disableFormTracking: scriptEl.getAttribute('data-disable-form-tracking') === 'true',
        enableDeclarativeEvents: scriptEl.getAttribute('data-enable-declarative-events') === 'true',
        enablePaidSignals: scriptEl.getAttribute('data-enable-paid-signals') === 'true',
        enableWebVitals: scriptEl.getAttribute('data-enable-web-vitals') === 'true',
        enableVisibleTime: scriptEl.getAttribute('data-enable-visible-time') === 'true',
        trackerSource: source,
        trackerVersion: version
    };
}

export function isTrackerBlocked(hostname: string, userAgent: string, doNotTrack: string | null | undefined, collectDnt: boolean, captureOnLocalhost = false): boolean {
    const isBot = /bot|spider|crawl|slurp|ia_archiver/i.test(userAgent);
    const isLocal = !captureOnLocalhost && (hostname === 'localhost' || hostname === '127.0.0.1');
    const dntEnabled = doNotTrack === '1';
    return isLocal || isBot || (dntEnabled && !collectDnt);
}

export function isTrackingOptedOut(win: Window): boolean {
    try {
        return win.localStorage.getItem(OPT_OUT_KEY) === 'true';
    } catch {
        return false;
    }
}

export function setTrackingOptOut(win: Window, optedOut: boolean): void {
    try {
        if (optedOut) {
            win.localStorage.setItem(OPT_OUT_KEY, 'true');
        } else {
            win.localStorage.removeItem(OPT_OUT_KEY);
        }
    } catch (error) {
        ignoreError(error);
    }
}

export function sanitizeTrackedUrl(rawUrl: string, baseUrl: string | URL): URL | null {
    try {
        const url = new URL(rawUrl, baseUrl);
        if (url.protocol !== 'http:' && url.protocol !== 'https:') {
            return null;
        }
        url.search = '';
        url.hash = '';
        return url;
    } catch {
        return null;
    }
}

export function sanitizeTrackedPath(url: URL): string {
    return url.pathname || '/';
}

export function resolveTrackerUrl(scriptUrl: URL, relativePath: string): string {
    const scriptDirectory = new URL('.', scriptUrl);
    const normalizedPath = relativePath.replace(/^\/+/, '');
    return new URL(normalizedPath, scriptDirectory).toString();
}

export function resolveWebVitalsBundleUrl(scriptUrl: URL, bundleName = 'hk-vitals.js'): string {
    return resolveTrackerUrl(scriptUrl, bundleName);
}

/**
 * Single source of truth for the tracker's wire paths, shared by the `hk.js` snippet
 * (resolving against its own script URL) and the npm package (resolving against `host`).
 */
export function resolveTrackerEndpoints(baseUrl: URL, enableWebVitals: boolean, enableVisibleTime: boolean): TrackerEndpoints {
    return {
        pageEndpoint: resolveTrackerUrl(baseUrl, 'ingest'),
        eventEndpoint: resolveTrackerUrl(baseUrl, 'ingest/event'),
        webVitalsEndpoint: resolveTrackerUrl(baseUrl, 'ingest/web-vitals'),
        visibleTimeEndpoint: enableVisibleTime ? resolveTrackerUrl(baseUrl, 'ingest/visible-time') : null,
        webVitalsBundleUrl: enableWebVitals ? resolveWebVitalsBundleUrl(baseUrl) : null
    };
}

function loadWebVitalsBundle(win: HitKeepWindow, bundleUrl: string): void {
    if (win.hk?._webVitalsLoaded) {
        return;
    }
    const script = win.document.createElement('script');
    script.async = true;
    script.referrerPolicy = 'no-referrer';
    script.src = bundleUrl;
    win.hk = win.hk || {};
    win.hk._webVitalsLoaded = true;
    win.document.head.appendChild(script);
}

export function extractDownloadExtension(url: URL): string | null {
    const lastSegment = sanitizeTrackedPath(url).split('/').pop() ?? '';
    const parts = lastSegment.split('.');
    if (parts.length < 2) {
        return null;
    }

    const extension = parts[parts.length - 1]?.toLowerCase() ?? '';
    return DOWNLOAD_EXTENSIONS.has(extension) ? extension : null;
}

export function hasExplicitTrackingTag(element: Element | null): boolean {
    return Boolean(element?.closest(EXPLICIT_TRACKING_SELECTORS));
}

export function classifyLinkEvent(link: HTMLAnchorElement | HTMLAreaElement, currentUrl: URL): HitKeepEvent | null {
    if (link.closest('form') || hasExplicitTrackingTag(link)) {
        return null;
    }

    const href = link.getAttribute('href');
    if (!href) {
        return null;
    }

    const targetUrl = sanitizeTrackedUrl(href, currentUrl);
    if (!targetUrl) {
        return null;
    }

    if (targetUrl.hostname !== currentUrl.hostname) {
        return {
            name: 'outbound_click',
            properties: {
                target_host: targetUrl.hostname,
                target_path: sanitizeTrackedPath(targetUrl),
                target_protocol: targetUrl.protocol.replace(':', '')
            }
        };
    }

    const fileExtension = link.hasAttribute('download') ? (extractDownloadExtension(targetUrl) ?? 'unknown') : extractDownloadExtension(targetUrl);
    if (!fileExtension) {
        return null;
    }

    return {
        name: 'file_download',
        properties: {
            file_host: targetUrl.hostname,
            file_path: sanitizeTrackedPath(targetUrl),
            file_ext: fileExtension
        }
    };
}

export function classifyFormSubmit(form: HTMLFormElement, currentUrl: URL, submitter?: Element | null): HitKeepEvent | null {
    return findFormMarker(form, submitter) ? null : formSubmitEvent(form, currentUrl);
}

function formSubmitEvent(form: HTMLFormElement, currentUrl: URL): HitKeepEvent | null {
    const rawAction = form.getAttribute('action')?.trim() || currentUrl.toString();
    const actionUrl = sanitizeTrackedUrl(rawAction, currentUrl);
    if (!actionUrl) {
        return null;
    }

    const method = (form.getAttribute('method')?.trim() || 'get').toLowerCase();
    const formId = form.getAttribute('id')?.trim();
    const properties: EventProperties = {
        action_host: actionUrl.hostname,
        action_path: sanitizeTrackedPath(actionUrl),
        method,
        same_origin: actionUrl.origin === currentUrl.origin
    };

    if (formId) {
        properties['form_id'] = formId;
    }

    return {
        name: 'form_submit',
        properties
    };
}

export function createTracker(win: HitKeepWindow, config: TrackerRuntimeConfig): TrackerHandle | null {
    const { document, location, navigator, screen, history, sessionStorage, crypto } = win;
    const prerenderDocument = document as PrerenderDocument;

    const hk = (win.hk ??= {});
    if (hk._bootstrapped) {
        return null;
    }
    hk._bootstrapped = true;
    let active = true;

    const bindHandle = (track: EventSender, trackPageview: () => void, getSessionId: () => string | undefined, dispose: () => void = noop): TrackerHandle => {
        if (config.bindToWindow) {
            hk.event = track;
            hk.getSessionId = getSessionId;
        }
        return {
            track,
            trackPageview,
            cleanup: () => {
                if (!active) return;
                active = false;
                dispose();
                if (config.bindToWindow) {
                    if (hk.event === track) delete hk.event;
                    if (hk.getSessionId === getSessionId) delete hk.getSessionId;
                }
                hk._bootstrapped = false;
            }
        };
    };

    if (isTrackerBlocked(location.hostname, navigator.userAgent, navigator.doNotTrack, config.collectDnt, config.captureOnLocalhost) || isTrackingOptedOut(win)) {
        return bindHandle(noop, noop, noop);
    }

    const generateUUID = () => {
        if (crypto?.randomUUID) {
            return crypto.randomUUID();
        }
        return '10000000-1000-4000-8000-100000000000'.replace(/[018]/g, (value) => (Number(value) ^ (crypto.getRandomValues(new Uint8Array(1))[0]! & (15 >> (Number(value) / 4)))).toString(16));
    };

    const getSessionId = () => {
        const now = Date.now();
        let sessionId: string | null = null;

        try {
            const stored = sessionStorage.getItem(SESSION_KEY);
            if (stored) {
                const [id, lastActive] = stored.split('|');
                if (id && lastActive && now - parseInt(lastActive, 10) < SESSION_EXPIRY) {
                    sessionId = id;
                }
            }
        } catch (error) {
            ignoreError(error);
        }

        if (!sessionId) {
            sessionId = generateUUID();
        }

        try {
            sessionStorage.setItem(SESSION_KEY, `${sessionId}|${now}`);
        } catch (error) {
            ignoreError(error);
        }

        return sessionId;
    };

    const sessionId = getSessionId();
    const initialReferrer = document.referrer.split(/[?#]/, 1)[0];
    const initialHost = location.hostname;
    let awaitingActivation = prerenderDocument.prerendering === true || (document.visibilityState as string) === 'prerender';
    const readUtmValue = (params: URLSearchParams, key: string) => {
        const value = params.get(key);
        if (!value) {
            return null;
        }
        const trimmed = value.trim();
        return trimmed.length > 0 ? trimmed : null;
    };
    const readAttribution = () => {
        const params = new URLSearchParams(location.search);
        return {
            u_src: readUtmValue(params, 'utm_source'),
            u_med: readUtmValue(params, 'utm_medium'),
            u_cmp: readUtmValue(params, 'utm_campaign'),
            u_trm: readUtmValue(params, 'utm_term'),
            u_cnt: readUtmValue(params, 'utm_content'),
            qr: readUtmValue(params, 'hk_qr'),
            pc: config.enablePaidSignals ? params.getAll('gclid').some(Boolean) || params.getAll('msclkid').some(Boolean) : undefined
        };
    };
    let initialAttribution = awaitingActivation ? null : readAttribution();
    const pendingRequests: PendingRequest[] = [];
    const pendingActivationRequests: PendingActivationRequest[] = [];
    const removers: (() => void)[] = [];
    const on = (target: EventTarget, type: string, listener: EventListener, options?: boolean | AddEventListenerOptions) => {
        target.addEventListener(type, listener, options);
        removers.push(() => target.removeEventListener(type, listener, options));
    };
    let retryTimer: ReturnType<typeof setTimeout> | null = null;
    let lastPath = location.pathname;
    let lastPageviewPath = '';
    let lastPageviewAt = 0;
    let currentPageId = generateUUID();
    let pendingPageview = false;
    const visibleTimeEndpoint = config.visibleTimeEndpoint;
    let visibleTimeView: VisibleTimeView | null = null;
    let pageFrozen = false;

    const canMeasureVisibleTime = () => !isTrackingOptedOut(win) && (config.collectDnt || navigator.doNotTrack !== '1');

    // Beacon cannot prevent the browser from putting the current query in Referer.
    const canSendBeacon = () => typeof navigator.sendBeacon === 'function' && !config.disableBeacon && !location.search;

    const resumeVisibleTime = () => {
        if (visibleTimeView && !pageFrozen && document.visibilityState === 'visible') {
            visibleTimeView.startedAt ??= win.performance.now();
        }
    };

    const checkpointVisibleTime = (final: boolean) => {
        const view = visibleTimeView;
        if (!view || !visibleTimeEndpoint) {
            return;
        }
        if (!canMeasureVisibleTime()) {
            visibleTimeView = null;
            return;
        }
        if (view.startedAt !== null) {
            view.elapsedMs += win.performance.now() - view.startedAt;
            view.startedAt = null;
        }
        if (final) {
            visibleTimeView = null;
        }
        const ms = Math.min(MAX_VISIBLE_TIME_MS, Math.floor(view.elapsedMs));
        if (ms === view.sentMs || (!final && view.checkpoints >= 2)) {
            return;
        }
        view.sentMs = ms;
        view.checkpoints++;

        // A string body sends JSON as CORS-safelisted plain text without a preflight.
        const body = JSON.stringify({ pid: currentPageId, ms });
        if (canSendBeacon() && navigator.sendBeacon(visibleTimeEndpoint, body)) {
            return;
        }
        fetch(visibleTimeEndpoint, { method: 'POST', body, keepalive: true, credentials: 'omit', referrerPolicy: 'no-referrer' }).catch(ignoreError);
    };

    const queueActivationRequest = (request: PendingActivationRequest) => {
        pendingActivationRequests.push(request);
        if (pendingActivationRequests.length > MAX_PENDING_ACTIVATION_REQUESTS) {
            pendingActivationRequests.splice(0, pendingActivationRequests.length - MAX_PENDING_ACTIVATION_REQUESTS);
        }
    };

    const queueRequest = (request: PendingRequest) => {
        pendingRequests.push(request);
        if (pendingRequests.length > MAX_PENDING_REQUESTS) {
            pendingRequests.splice(0, pendingRequests.length - MAX_PENDING_REQUESTS);
        }
        scheduleFlush();
    };

    const flushQueue = () => {
        if (pendingRequests.length === 0) {
            return;
        }
        if (retryTimer !== null) {
            clearTimeout(retryTimer);
            retryTimer = null;
        }

        const requests = pendingRequests.splice(0, pendingRequests.length);
        for (const request of requests) {
            sendRequest(request, true);
        }
    };

    const scheduleFlush = () => {
        if (retryTimer !== null || pendingRequests.length === 0) {
            return;
        }

        retryTimer = setTimeout(() => {
            retryTimer = null;
            flushQueue();
        }, RETRY_DELAY_MS);
    };

    const sendRequest = (request: PendingRequest, fromQueue = false) => {
        if (!active || (request.respectOptOut && isTrackingOptedOut(win))) {
            return;
        }
        const headers = { 'Content-Type': 'application/json' };

        if (canSendBeacon()) {
            const blob = new Blob([request.body], { type: 'application/json' });
            if (navigator.sendBeacon(request.endpoint, blob)) {
                return;
            }
        }

        fetch(request.endpoint, {
            method: 'POST',
            body: request.body,
            headers,
            keepalive: true,
            credentials: 'omit',
            referrerPolicy: 'no-referrer'
        })
            .then((response) => {
                if (!response.ok) {
                    throw new Error(`tracker_request_failed_${response.status}`);
                }
            })
            .catch((error) => {
                ignoreError(error);
                if (!fromQueue) {
                    queueRequest(request);
                } else {
                    pendingRequests.unshift(request);
                    if (pendingRequests.length > MAX_PENDING_REQUESTS) {
                        pendingRequests.length = MAX_PENDING_REQUESTS;
                    }
                    scheduleFlush();
                }
            });
    };

    const sendJson = (endpoint: string, payload: object, respectOptOut?: boolean) => {
        sendRequest({
            endpoint,
            body: JSON.stringify(payload),
            respectOptOut
        });
    };

    const currentReferrer = () => {
        if (lastPath !== location.pathname) {
            return `${location.origin}${lastPath}`;
        }
        return initialReferrer || null;
    };

    const emitEvent = (name: string, properties: EventProperties = {}, declarative?: boolean) => {
        const payload: EventPayload = {
            n: name,
            p: properties,
            r: currentReferrer(),
            sid: sessionId,
            path: location.pathname || '/',
            ua: navigator.userAgent,
            tsrc: config.trackerSource,
            tv: config.trackerVersion
        };
        if (awaitingActivation) {
            if (declarative && isTrackingOptedOut(win)) {
                return;
            }
            queueActivationRequest({ kind: 'event', endpoint: config.eventEndpoint, payload: JSON.parse(JSON.stringify(payload)) as EventPayload, declarative });
            return;
        }
        sendJson(config.eventEndpoint, payload, declarative);
    };

    if (config.webVitalsBundleUrl) {
        const context: WebVitalsTrackerContext = {
            emit: (payload) => {
                if (awaitingActivation) {
                    queueActivationRequest({ kind: 'web-vital', endpoint: config.webVitalsEndpoint, payload: { ...payload } });
                    return;
                }
                sendJson(config.webVitalsEndpoint, payload);
            },
            getPath: () => location.pathname || '/',
            sessionId,
            pageId: () => currentPageId,
            trackerSource: config.trackerSource,
            trackerVersion: config.trackerVersion,
            userAgent: navigator.userAgent
        };
        hk._webVitals = context;
        removers.push(() => {
            if (hk._webVitals === context) delete hk._webVitals;
        });
        loadWebVitalsBundle(win, config.webVitalsBundleUrl);
    }

    const sendPageViewNow = () => {
        const currentPath = location.pathname;
        const now = Date.now();

        try {
            sessionStorage.setItem(SESSION_KEY, `${sessionId}|${now}`);
        } catch (error) {
            ignoreError(error);
        }

        if (currentPath === lastPageviewPath && now - lastPageviewAt < DUPLICATE_PAGEVIEW_WINDOW_MS) {
            lastPath = currentPath;
            return;
        }

        const referrer = currentReferrer();
        const isUnique = lastPath === currentPath && referrer ? new URL(referrer, location.href).hostname !== initialHost : false;
        if (initialAttribution === null) {
            initialAttribution = readAttribution();
        }

        checkpointVisibleTime(true);
        currentPageId = generateUUID();
        const payload = {
            path: currentPath,
            referrer: referrer || null,
            ua: navigator.userAgent,
            vp_w: win.innerWidth,
            vp_h: win.innerHeight,
            sc_w: screen.width,
            sc_h: screen.height,
            lang: navigator.language,
            ...initialAttribution,
            unique: Boolean(isUnique),
            session_id: sessionId,
            page_id: currentPageId,
            tsrc: config.trackerSource,
            tv: config.trackerVersion
        };
        sendJson(config.pageEndpoint, payload, config.enablePaidSignals);
        if (visibleTimeEndpoint && canMeasureVisibleTime()) {
            visibleTimeView = { startedAt: null, elapsedMs: 0, sentMs: -1, checkpoints: 0 };
            resumeVisibleTime();
        }

        lastPageviewPath = currentPath;
        lastPageviewAt = now;
        lastPath = currentPath;
    };

    const sendPageView = () => {
        if (!active) {
            return;
        }
        if (awaitingActivation) {
            pendingPageview = true;
            return;
        }
        sendPageViewNow();
    };

    const activate = () => {
        if (!awaitingActivation) {
            return;
        }
        awaitingActivation = false;
        initialAttribution = readAttribution();
        lastPath = location.pathname;
        if (pendingPageview) {
            pendingPageview = false;
            sendPageViewNow();
        }
        const requests = pendingActivationRequests.splice(0, pendingActivationRequests.length);
        for (const request of requests) {
            if (request.kind === 'event') {
                sendJson(
                    request.endpoint,
                    {
                        ...request.payload,
                        r: currentReferrer(),
                        path: location.pathname || '/'
                    },
                    request.declarative
                );
                continue;
            }
            sendJson(request.endpoint, {
                ...request.payload,
                p: location.pathname || '/',
                pid: currentPageId
            });
        }
    };

    if (prerenderDocument.prerendering === true) {
        const handlePrerenderActivation = () => {
            if (!awaitingActivation || prerenderDocument.prerendering === true) {
                return;
            }
            activate();
        };
        on(document, 'prerenderingchange', handlePrerenderActivation, { once: true });
    } else if ((document.visibilityState as string) === 'prerender') {
        const handleLegacyPrerenderActivation = () => {
            if (!awaitingActivation || document.visibilityState !== 'visible') {
                return;
            }
            activate();
        };
        on(document, 'visibilitychange', handleLegacyPrerenderActivation);
    }

    const patchHistoryMethod = (method: HistoryMethod) => {
        const original = history[method];
        const patched = function patchedHistory(this: History, ...args: Parameters<History[HistoryMethod]>) {
            original.apply(this, args);
            sendPageView();
        };
        history[method] = patched;
        removers.push(() => {
            if (history[method] === patched) {
                history[method] = original;
            }
        });
    };

    if (!config.disableSpaTracking) {
        patchHistoryMethod('pushState');
        patchHistoryMethod('replaceState');

        on(win, 'popstate', sendPageView);
        on(win, 'hashchange', sendPageView);
    }

    const handleVisibilityFlush = () => {
        if (document.visibilityState === 'hidden') {
            flushQueue();
        }
    };
    on(win, 'online', flushQueue);
    on(win, 'pagehide', flushQueue);
    on(document, 'visibilitychange', handleVisibilityFlush);

    if (visibleTimeEndpoint) {
        on(document, 'visibilitychange', () => {
            if (document.visibilityState === 'visible') {
                resumeVisibleTime();
            } else {
                checkpointVisibleTime(false);
            }
        });
        on(win, 'pagehide', (event) => {
            pageFrozen = true;
            checkpointVisibleTime(!(event as PageTransitionEvent).persisted);
        });
        on(win, 'pageshow', (event) => {
            if ((event as PageTransitionEvent).persisted) {
                pageFrozen = false;
                resumeVisibleTime();
            }
        });
    }

    if (config.capturePageviews) {
        sendPageView();
    }

    bindAutoTracking(document, () => new URL(location.href), config, emitEvent, on);

    const track: EventSender = (name, properties) => emitEvent(name, properties ?? {});
    const currentSessionId = () => (active && !awaitingActivation && !isTrackingOptedOut(win) ? sessionId : undefined);
    return bindHandle(track, sendPageView, currentSessionId, () => {
        visibleTimeView = null;
        pendingPageview = false;
        pendingActivationRequests.length = 0;
        if (retryTimer !== null) {
            clearTimeout(retryTimer);
            retryTimer = null;
        }
        for (const remove of removers.splice(0, removers.length)) {
            remove();
        }
    });
}

export function bootstrapTracker(win: HitKeepWindow = window): void {
    const scriptEl = resolveTrackerScript(win.document);
    if (!scriptEl) {
        return;
    }

    const options = readTrackerOptions(scriptEl);
    const scriptUrl = new URL(scriptEl.src, win.location.href);
    createTracker(win, {
        ...options,
        ...resolveTrackerEndpoints(scriptUrl, options.enableWebVitals, options.enableVisibleTime),
        capturePageviews: true,
        captureOnLocalhost: false,
        bindToWindow: true
    });
}

function resolveTrackerScript(document: Document): HTMLScriptElement | null {
    const currentScript = document.currentScript;
    if (currentScript instanceof HTMLScriptElement) {
        return currentScript;
    }

    const script = document.querySelector('script[src*="hk.js"]');
    return script instanceof HTMLScriptElement ? script : null;
}

function emitMarkedEvent(marker: Element, emit: InteractionEventSender): void {
    const name = marker.getAttribute('data-hk-event') ?? marker.getAttribute('data-hitkeep-event');
    if (name && /^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/.test(name)) {
        emit(name, undefined, true);
    }
}

function findFormMarker(form: HTMLFormElement, submitter?: Element | null): Element | null {
    return submitter?.closest(EXPLICIT_TRACKING_SELECTORS) ?? form.closest(EXPLICIT_TRACKING_SELECTORS);
}

function findClickMarker(target: Element): Element | null {
    const marker = target.closest(EXPLICIT_TRACKING_SELECTORS);
    return marker?.contains(target.closest('form')) ? null : marker;
}

function shouldDeferNamedClick(target: Element): boolean {
    const clicked = target.closest(CLICK_CONTROL_SELECTORS);
    const label = clicked instanceof HTMLLabelElement ? clicked : null;
    const control = label ? label.control : clicked;
    // A submit button's event comes from submission, even when activated by a label.
    if ((control instanceof HTMLButtonElement || control instanceof HTMLInputElement) && control.form !== null && (control.type === 'submit' || control.type === 'image')) {
        return true;
    }
    // A label forwards a second click to its control, which can already record the event.
    return label !== null && control !== null && !control.matches(':disabled') && findClickMarker(control) !== null;
}

function bindAutoTracking(document: Document, getCurrentUrl: () => URL, options: TrackerOptions, emitEvent: InteractionEventSender, on: ListenerRegistrar): void {
    const { enableDeclarativeEvents, disableFormTracking } = options;
    const trackLinks = !options.disableOutboundTracking || !options.disableDownloadTracking;
    if (trackLinks || enableDeclarativeEvents) {
        const handleLinkInteraction = (event: MouseEvent) => {
            if ((event.type === 'click' && event.button !== 0) || (event.type === 'auxclick' && event.button !== 1)) {
                return;
            }

            const target = event.target;
            if (!(target instanceof Element)) {
                return;
            }

            if (enableDeclarativeEvents) {
                const marker = findClickMarker(target);
                if (marker) {
                    if (event.type === 'click' ? !shouldDeferNamedClick(target) : target.closest(LINK_SELECTORS)) {
                        emitMarkedEvent(marker, emitEvent);
                    }
                    return;
                }
                if (!trackLinks) {
                    return;
                }
            }

            const link = target.closest(LINK_SELECTORS);
            if (!(link instanceof HTMLAnchorElement || link instanceof HTMLAreaElement)) {
                return;
            }

            const trackedEvent = classifyLinkEvent(link, getCurrentUrl());
            if (!trackedEvent) {
                return;
            }

            if (trackedEvent.name === 'outbound_click' && options.disableOutboundTracking) {
                return;
            }

            if (trackedEvent.name === 'file_download' && options.disableDownloadTracking) {
                return;
            }

            emitEvent(trackedEvent.name, trackedEvent.properties);
        };

        on(document, 'click', handleLinkInteraction as EventListener, true);
        on(document, 'auxclick', handleLinkInteraction as EventListener, true);
    }

    if (!disableFormTracking || enableDeclarativeEvents) {
        const handleFormSubmit = (event: Event) => {
            const target = event.target;
            if (!(target instanceof HTMLFormElement)) {
                return;
            }

            const candidate = (event as SubmitEvent).submitter;
            const submitter = candidate instanceof Element ? candidate : null;
            const marker = findFormMarker(target, submitter);
            if (marker) {
                if (enableDeclarativeEvents) {
                    emitMarkedEvent(marker, emitEvent);
                }
                return;
            }
            if (disableFormTracking) {
                return;
            }
            const trackedEvent = formSubmitEvent(target, getCurrentUrl());
            if (!trackedEvent) {
                return;
            }

            emitEvent(trackedEvent.name, trackedEvent.properties);
        };

        on(document, 'submit', handleFormSubmit, true);
    }
}
