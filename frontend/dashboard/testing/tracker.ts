import { vi } from 'vitest';

import type { HitKeepWindow } from '../src/tracker/core';

type ListenerMap = Record<string, EventListener[]>;

export function trackerHarness(path = '/signup', scriptSrc: string | null = 'https://analytics.example.com/hk.js') {
    const script = document.createElement('script');
    if (scriptSrc) script.src = scriptSrc;
    const appendedScripts: HTMLScriptElement[] = [];

    let currentUrl = new URL(path, 'https://app.example.com');
    const location = {
        get href() {
            return currentUrl.href;
        },
        get hostname() {
            return currentUrl.hostname;
        },
        set hostname(value: string) {
            currentUrl.hostname = value;
        },
        get pathname() {
            return currentUrl.pathname;
        },
        get search() {
            return currentUrl.search;
        },
        get origin() {
            return currentUrl.origin;
        }
    } as Location;

    const windowListeners: ListenerMap = {};
    const documentListeners: ListenerMap = {};
    let visibilityState = 'visible';
    let prerendering = false;
    let monotonicTime = 0;
    const stored = new Map<string, string>();
    const sessionStorage = {
        getItem: vi.fn((key: string) => stored.get(key) ?? null),
        setItem: vi.fn((key: string, value: string) => {
            stored.set(key, value);
        })
    };
    const persisted = new Map<string, string>();
    const localStorage = {
        getItem: vi.fn((key: string) => persisted.get(key) ?? null),
        setItem: vi.fn((key: string, value: string) => {
            persisted.set(key, value);
        }),
        removeItem: vi.fn((key: string) => {
            persisted.delete(key);
        })
    };
    const sendBeacon = vi.fn((url: string | URL, data?: BodyInit | null) => {
        void url;
        void data;
        return true;
    });
    const history = {
        pushState: vi.fn((_state: unknown, _title: string, url?: string | URL | null) => {
            if (url) {
                currentUrl = new URL(url, currentUrl.href);
            }
        }),
        replaceState: vi.fn((_state: unknown, _title: string, url?: string | URL | null) => {
            if (url) {
                currentUrl = new URL(url, currentUrl.href);
            }
        })
    };
    const fakeDocument = {
        currentScript: scriptSrc ? script : null,
        referrer: scriptSrc ? 'https://referrer.example.com/article?secret=1#fragment' : '',
        get visibilityState() {
            return visibilityState;
        },
        get prerendering() {
            return prerendering;
        },
        createElement: vi.fn((tagName: string) => document.createElement(tagName)),
        head: {
            appendChild: vi.fn((element: HTMLScriptElement) => {
                appendedScripts.push(element);
                return element;
            })
        },
        querySelector: vi.fn(() => (scriptSrc ? script : null)),
        addEventListener: vi.fn((name: string, listener: EventListener) => {
            documentListeners[name] = [...(documentListeners[name] ?? []), listener];
        }),
        removeEventListener: vi.fn((name: string, listener: EventListener) => {
            documentListeners[name] = (documentListeners[name] ?? []).filter((existing) => existing !== listener);
        })
    };
    const win = {
        document: fakeDocument,
        location,
        navigator: {
            userAgent: 'Mozilla/5.0',
            doNotTrack: '0',
            language: 'en-US',
            sendBeacon
        },
        screen: { width: 1440, height: 900 },
        history,
        performance: { now: () => monotonicTime },
        sessionStorage,
        localStorage,
        crypto: {
            randomUUID: vi.fn(() => '10000000-0000-4000-8000-000000000001'),
            getRandomValues: crypto.getRandomValues.bind(crypto)
        },
        innerWidth: 1280,
        innerHeight: 720,
        addEventListener: vi.fn((name: string, listener: EventListener) => {
            windowListeners[name] = [...(windowListeners[name] ?? []), listener];
        }),
        removeEventListener: vi.fn((name: string, listener: EventListener) => {
            windowListeners[name] = (windowListeners[name] ?? []).filter((existing) => existing !== listener);
        }),
        hk: undefined
    } as unknown as HitKeepWindow;

    return {
        documentListeners,
        appendedScripts,
        fakeDocument,
        history,
        localStorage,
        script,
        sendBeacon,
        sessionStorage,
        setTime: (next: number) => {
            monotonicTime = next;
        },
        setVisibilityState: (next: string) => {
            visibilityState = next;
        },
        setPrerendering: (next: boolean) => {
            prerendering = next;
        },
        win,
        windowListeners
    };
}

export function packageHarness(path = '/pricing') {
    return trackerHarness(path, null);
}

export async function beaconPayload(sendBeacon: ReturnType<typeof packageHarness>['sendBeacon'], callIndex: number): Promise<Record<string, unknown>> {
    const body = sendBeacon.mock.calls[callIndex]?.[1] as unknown as Blob;
    return JSON.parse(await body.text()) as Record<string, unknown>;
}
