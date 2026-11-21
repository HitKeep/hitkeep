import { formatComparisonPeriod, previousPeriod } from '@core/analytics/comparison-period';

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
    });
});
