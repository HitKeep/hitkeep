import { DEFAULT_RANGE_OPTIONS, DEFAULT_RANGE_VALUE, isShortRange, resolveDateRange, selectDefaultRange } from './range-options';

describe('range options', () => {
    const now = new Date(2026, 6, 3, 15, 45, 30, 250);

    it('selects today when no previous range exists', () => {
        const selected = selectDefaultRange(DEFAULT_RANGE_OPTIONS);

        expect(DEFAULT_RANGE_VALUE).toBe('today');
        expect(selected.value).toBe('today');
    });

    it('keeps common report presets available while defaulting to today first', () => {
        const values = DEFAULT_RANGE_OPTIONS.map((range) => range.value);
        expect(values).toEqual(['today', 'yesterday', '24h', '7d', '28d', '30m', '1h', '6h', '3d', '14d', '30d', '60d', '90d', '180d', 'thisWeek', 'lastWeek', 'thisMonth', 'lastMonth', 'thisYear', '1y', 'custom']);
    });

    it('resolves today from local midnight to now', () => {
        expect(resolveDateRange({ value: 'today' }, null, now)).toEqual({
            from: new Date(2026, 6, 3, 0, 0, 0, 0).toISOString(),
            to: now.toISOString()
        });
    });

    it('resolves yesterday to the previous local calendar day', () => {
        expect(resolveDateRange({ value: 'yesterday' }, null, now)).toEqual({
            from: new Date(2026, 6, 2, 0, 0, 0, 0).toISOString(),
            to: new Date(2026, 6, 2, 23, 59, 59, 999).toISOString()
        });
    });

    it('resolves rolling minute, hour, and day presets to now', () => {
        expect(resolveDateRange({ value: '30m' }, null, now)).toEqual({
            from: new Date(2026, 6, 3, 15, 15, 30, 250).toISOString(),
            to: now.toISOString()
        });
        expect(resolveDateRange({ value: '1h' }, null, now)).toEqual({
            from: new Date(2026, 6, 3, 14, 45, 30, 250).toISOString(),
            to: now.toISOString()
        });
        expect(resolveDateRange({ value: '6h' }, null, now)).toEqual({
            from: new Date(2026, 6, 3, 9, 45, 30, 250).toISOString(),
            to: now.toISOString()
        });
        expect(resolveDateRange({ value: '3d' }, null, now)).toEqual({
            from: new Date(2026, 5, 30, 15, 45, 30, 250).toISOString(),
            to: now.toISOString()
        });
        expect(resolveDateRange({ value: '14d' }, null, now)).toEqual({
            from: new Date(2026, 5, 19, 15, 45, 30, 250).toISOString(),
            to: now.toISOString()
        });
        expect(resolveDateRange({ value: '28d' }, null, now)).toEqual({
            from: new Date(2026, 5, 5, 15, 45, 30, 250).toISOString(),
            to: now.toISOString()
        });
        expect(resolveDateRange({ value: '30d' }, null, now)).toEqual({
            from: new Date(2026, 5, 3, 15, 45, 30, 250).toISOString(),
            to: now.toISOString()
        });
        expect(resolveDateRange({ value: '60d' }, null, now)).toEqual({
            from: new Date(2026, 4, 4, 15, 45, 30, 250).toISOString(),
            to: now.toISOString()
        });
        expect(resolveDateRange({ value: '180d' }, null, now)).toEqual({
            from: new Date(2026, 0, 4, 15, 45, 30, 250).toISOString(),
            to: now.toISOString()
        });
    });

    describe(`elapsed presets across DST in ${Intl.DateTimeFormat().resolvedOptions().timeZone}`, () => {
        for (const [value, duration] of [
            ['30m', 30 * 60 * 1000],
            ['1h', 60 * 60 * 1000],
            ['6h', 6 * 60 * 60 * 1000],
            ['24h', 24 * 60 * 60 * 1000]
        ] as const) {
            it.each(['2026-03-29T01:10:00.123Z', '2026-10-25T01:30:00.123Z', '2026-03-08T07:10:00.123Z', '2026-11-01T06:30:00.123Z'])(`keeps ${value} at its exact elapsed duration ending at %s`, (timestamp) => {
                const instant = new Date(timestamp);
                const range = resolveDateRange({ value }, null, instant)!;

                expect(new Date(range.to).getTime() - new Date(range.from).getTime()).toBe(duration);
                expect(range.to).toBe(timestamp);
                expect(instant.toISOString()).toBe(timestamp);
            });
        }
    });

    it('resolves local calendar week, month, and year presets', () => {
        expect(resolveDateRange({ value: 'thisWeek' }, null, now)).toEqual({
            from: new Date(2026, 5, 29, 0, 0, 0, 0).toISOString(),
            to: now.toISOString()
        });
        expect(resolveDateRange({ value: 'lastWeek' }, null, now)).toEqual({
            from: new Date(2026, 5, 22, 0, 0, 0, 0).toISOString(),
            to: new Date(2026, 5, 28, 23, 59, 59, 999).toISOString()
        });
        expect(resolveDateRange({ value: 'thisMonth' }, null, now)).toEqual({
            from: new Date(2026, 6, 1, 0, 0, 0, 0).toISOString(),
            to: now.toISOString()
        });
        expect(resolveDateRange({ value: 'lastMonth' }, null, now)).toEqual({
            from: new Date(2026, 5, 1, 0, 0, 0, 0).toISOString(),
            to: new Date(2026, 5, 30, 23, 59, 59, 999).toISOString()
        });
        expect(resolveDateRange({ value: 'thisYear' }, null, now)).toEqual({
            from: new Date(2026, 0, 1, 0, 0, 0, 0).toISOString(),
            to: now.toISOString()
        });
    });

    it('treats sub-day ranges as short chart ranges', () => {
        expect(isShortRange({ value: '30m' })).toBe(true);
        expect(isShortRange({ value: '1h' })).toBe(true);
        expect(isShortRange({ value: '6h' })).toBe(true);
        expect(isShortRange({ value: '3d' })).toBe(false);
    });

    it('rejects incomplete or inverted custom ranges', () => {
        expect(resolveDateRange({ value: 'custom' }, null, now)).toBeNull();
        expect(resolveDateRange({ value: 'custom' }, [new Date(2026, 6, 3), new Date(2026, 6, 2)], now)).toBeNull();
    });
});
