import { formatCompactComparisonPeriod, formatComparisonPeriod, previousPeriod } from '@core/analytics/comparison-period';

describe('report comparison periods', () => {
    it('places a 30-minute baseline immediately before the selected window', () => {
        const previous = previousPeriod('2027-01-01T00:10:00.000Z', '2027-01-01T00:40:00.000Z');
        expect(previous).toEqual({ from: '2026-12-31T23:39:59.999Z', to: '2027-01-01T00:09:59.999Z' });

        const label = formatComparisonPeriod(previous, 'en-GB', 'UTC');
        expect(label).toContain('31 Dec 2026');
        expect(label).toContain('1 Jan 2027');
        expect(label).toContain('23:39:59.999');
        expect(label).toContain('00:09:59.999');
    });

    it('keeps elapsed time through DST and formats each endpoint with its own offset', () => {
        const current = { from: '2026-03-28T23:00:00.000Z', to: '2026-03-29T21:59:59.999Z' };
        const previous = previousPeriod(current.from, current.to);
        expect(previous).toEqual({ from: '2026-03-28T00:00:00.000Z', to: '2026-03-28T22:59:59.999Z' });

        const english = formatComparisonPeriod(current, 'en-GB', 'Europe/Berlin');
        expect(english).toMatch(/GMT\+1.*GMT\+2/);
        expect(english).toContain('29 Mar 2026');
        const french = formatComparisonPeriod(current, 'fr-FR', 'Europe/Berlin');
        expect(french).toContain('29 mars 2026');
        expect(french).toContain('23:59:59,999');
        expect(formatCompactComparisonPeriod(current, 'en-GB', 'Europe/Berlin')).toBe('29 Mar 2026');
        expect(formatCompactComparisonPeriod(previous, 'en-GB', 'Europe/Berlin')).toMatch(/01:00.*23:59/);
    });

    it('omits clocks only when both inclusive endpoints cover full local calendar days', () => {
        const from = '2026-11-11T23:00:00.000Z';
        const to = '2026-11-18T22:59:59.999Z';
        const format = (start: string, end: string) => formatCompactComparisonPeriod({ from: start, to: end }, 'de-DE', 'Europe/Berlin');

        expect(format(from, to)).toMatch(/12\D+18\D+Nov\D+2026/);
        expect(format(from, to)).not.toContain(':');
        expect(format('2026-11-12T05:00:00.000Z', '2026-11-13T04:59:59.999Z')).toMatch(/12\..*06:00.*13\..*05:59/);
        expect(format(from, '2026-11-18T22:59:59.998Z')).toContain('23:59');
    });

    it('shows both calendar dates and years across a partial midnight boundary', () => {
        const period = { from: '2026-12-31T23:40:00.000Z', to: '2027-01-01T00:10:00.000Z' };
        expect(formatCompactComparisonPeriod(period, 'en-GB', 'UTC')).toMatch(/31 Dec 2026.*23:40.*1 Jan 2027.*00:10/);
    });

    it('retains distinct offsets and fractional seconds when compact times overlap', () => {
        const period = { from: '2026-10-25T00:30:20.123Z', to: '2026-10-25T01:30:20.456Z' };
        expect(formatCompactComparisonPeriod(period, 'de-DE', 'Europe/Berlin')).toContain('02:30');
        expect(formatComparisonPeriod(period, 'de-DE', 'Europe/Berlin')).toMatch(/02:30:20,123 GMT\+2.*02:30:20,456 GMT\+1/);

        const backwardClock = { from: '2026-10-25T00:50:00Z', to: '2026-10-25T01:10:00Z' };
        expect(formatComparisonPeriod(backwardClock, 'de-DE', 'Europe/Berlin')).toMatch(/02:50:00 GMT\+2.*02:10:00 GMT\+1/);
    });
});
