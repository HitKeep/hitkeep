import { DOCUMENT } from '@angular/common';
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Subject } from 'rxjs';
import { vi } from 'vitest';

import { RealtimeRefreshCoordinator } from '@services/realtime-refresh-coordinator.service';
import { RealtimeEvent, RealtimeService } from '@services/realtime.service';

describe('RealtimeRefreshCoordinator', () => {
    let events: Subject<RealtimeEvent>;
    let isOpen: ReturnType<typeof signal<boolean>>;
    let activeSiteId: ReturnType<typeof signal<string | null>>;

    beforeEach(() => {
        vi.useFakeTimers();
        events = new Subject<RealtimeEvent>();
        isOpen = signal(true);
        activeSiteId = signal('site-1');
        TestBed.configureTestingModule({
            providers: [
                {
                    provide: RealtimeService,
                    useValue: {
                        events$: events.asObservable(),
                        isOpen,
                        activeSiteId
                    }
                },
                { provide: DOCUMENT, useValue: document }
            ]
        });
    });

    afterEach(() => {
        TestBed.resetTestingModule();
        vi.useRealTimers();
    });

    it('coalesces continuous matching events without postponing refresh indefinitely', () => {
        const coordinator = TestBed.inject(RealtimeRefreshCoordinator);
        const refresh = vi.fn();

        coordinator.register({
            siteId: () => 'site-1',
            kinds: ['hits'],
            refresh,
            debounceMs: 500
        });

        for (let i = 0; i < 20; i++) {
            events.next(changeEvent({ site_id: 'site-1', kinds: ['hits'] }));
            vi.advanceTimersByTime(100);
        }
        expect(refresh).toHaveBeenCalledTimes(4);
    });

    it('ignores unrelated sites and kinds', () => {
        const coordinator = TestBed.inject(RealtimeRefreshCoordinator);
        const refresh = vi.fn();

        coordinator.register({
            siteId: () => 'site-1',
            kinds: ['hits'],
            refresh,
            debounceMs: 1
        });

        events.next(changeEvent({ site_id: 'site-2', kinds: ['hits'] }));
        events.next(changeEvent({ site_id: 'site-1', kinds: ['events'] }));
        vi.runOnlyPendingTimers();

        expect(refresh).not.toHaveBeenCalled();
    });

    it('falls back to centralized polling only when the realtime stream is unavailable', () => {
        const coordinator = TestBed.inject(RealtimeRefreshCoordinator);
        const refresh = vi.fn();

        coordinator.register({
            siteId: () => 'site-1',
            kinds: ['hits'],
            refresh,
            debounceMs: 1
        });

        vi.advanceTimersByTime(30000);
        vi.runOnlyPendingTimers();
        expect(refresh).not.toHaveBeenCalled();

        isOpen.set(false);
        vi.advanceTimersByTime(30000);
        vi.runOnlyPendingTimers();
        expect(refresh).toHaveBeenCalledTimes(1);
    });
});

function changeEvent(overrides: Partial<RealtimeEvent>): RealtimeEvent {
    return {
        type: 'analytics.changed',
        site_id: 'site-1',
        kinds: ['hits'],
        changed_at: '2026-06-01T12:00:00Z',
        bucket_start: '2026-06-01T12:00:00Z',
        counts: {},
        ...overrides
    };
}
