import { DOCUMENT } from '@angular/common';
import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import { EmbedService } from './embed.service';

describe('embedded analytics credential handoff', () => {
    const parent = { postMessage: vi.fn() };
    let listener: (event: MessageEvent) => void;
    let embed: EmbedService;
    const launch = (overrides = {}) => `header.${btoa(JSON.stringify({ site_id: 'site-a', parent_origin: 'https://wordpress.example', exp: Math.floor(Date.now() / 1000) + 300, ...overrides }))}.signature`;
    const message = (overrides = {}) =>
        ({
            source: parent,
            origin: 'https://wordpress.example',
            data: { type: 'hitkeep:embed:token', site_id: 'site-a', token: 'hk_embed_secret', expires_at: new Date(Date.now() + 300000).toISOString(), nonce: parent.postMessage.mock.lastCall?.[0].nonce },
            ...overrides
        }) as unknown as MessageEvent;

    beforeEach(() => {
        vi.useFakeTimers();
        parent.postMessage.mockClear();
        TestBed.configureTestingModule({
            providers: [
                {
                    provide: DOCUMENT,
                    useValue: {
                        defaultView: {
                            self: {},
                            top: parent,
                            parent,
                            crypto: { randomUUID: () => 'fresh-nonce' },
                            addEventListener: (_: string, callback: typeof listener) => {
                                listener = callback;
                            },
                            removeEventListener: vi.fn()
                        }
                    }
                }
            ]
        });
        embed = TestBed.inject(EmbedService);
    });

    afterEach(() => {
        TestBed.resetTestingModule();
        vi.useRealTimers();
    });

    it('accepts credentials only from the exact parent, origin, site and fresh challenge', () => {
        expect(embed.start('site-a', launch())).toBe(true);
        expect(parent.postMessage).toHaveBeenCalledWith({ type: 'hitkeep:embed:ready', site_id: 'site-a', nonce: 'fresh-nonce' }, 'https://wordpress.example');
        for (const invalid of [message({ source: {} }), message({ origin: 'https://evil.example' }), message({ data: { ...message().data, site_id: 'site-b' } }), message({ data: { ...message().data, nonce: 'stale' } })]) {
            listener(invalid);
            expect(embed.credential()).toBeNull();
        }
        listener(message());
        expect(embed.credential()?.token).toBe('hk_embed_secret');
        expect(embed.status()).toBe('ready');
    });

    it('refuses expired launches and mismatched routes before requesting credentials', () => {
        expect(embed.start('site-b', launch())).toBe(false);
        expect(embed.start('site-a', launch({ exp: 1 }))).toBe(false);
        expect(embed.start('site-a', launch({ parent_origin: 'https://wordpress.example/path' }))).toBe(false);
        expect(parent.postMessage).not.toHaveBeenCalled();
    });

    it('asks the parent to renew and discards credentials at expiry', () => {
        embed.start('site-a', launch());
        listener(message());
        vi.advanceTimersByTime(270000);
        expect(parent.postMessage).toHaveBeenCalledTimes(2);
        vi.advanceTimersByTime(30000);
        expect(embed.credential()).toBeNull();
        expect(embed.status()).toBe('error');
    });

    it('clears credentials and removes the listener when the shell closes', () => {
        embed.start('site-a', launch());
        listener(message());
        embed.stop();
        expect(embed.credential()).toBeNull();
        expect(embed.isEmbedMode()).toBe(false);
    });
});
