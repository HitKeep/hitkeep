export interface ComparisonPeriod {
    from: string;
    to: string;
}

/** Both endpoints are inclusive. Resolve calendar presets before calling this. */
export function previousPeriod(from: string, to: string): ComparisonPeriod {
    const start = Date.parse(from);
    const duration = Date.parse(to) - start;
    const end = start - 1;
    return { from: new Date(end - duration).toISOString(), to: new Date(end).toISOString() };
}

export function isIntradayPeriod(period: ComparisonPeriod | null): boolean {
    return !!period && Date.parse(period.to) - Date.parse(period.from) < 48 * 60 * 60 * 1000;
}

export function formatCompactComparisonPeriod(period: ComparisonPeriod, locale: string, timeZone?: string): string {
    const from = new Date(period.from);
    const to = new Date(period.to);
    const dateOptions = { timeZone, year: 'numeric', month: 'short', day: 'numeric' } as const;
    const dates = new Intl.DateTimeFormat(locale, dateOptions);
    // At an inclusive calendar boundary, the adjacent millisecond belongs to a different local date, even across DST.
    if (dates.format(from) !== dates.format(from.getTime() - 1) && dates.format(to) !== dates.format(to.getTime() + 1)) {
        return dates.formatRange(from, to);
    }

    return new Intl.DateTimeFormat(locale, { ...dateOptions, hour: '2-digit', minute: '2-digit' }).formatRange(from, to);
}

export function formatComparisonPeriod(period: ComparisonPeriod, locale: string, timeZone?: string): string {
    const from = new Date(period.from);
    const to = new Date(period.to);
    const fractionalSecondDigits = from.getMilliseconds() || to.getMilliseconds() ? 3 : undefined;
    const formatter = new Intl.DateTimeFormat(locale, {
        timeZone,
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        fractionalSecondDigits,
        timeZoneName: 'shortOffset'
    });
    // Formatting endpoints separately preserves both offsets when clocks change.
    return `${formatter.format(from)} – ${formatter.format(to)}`;
}
