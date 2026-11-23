export type TextDirection = 'ltr' | 'rtl';

const RTL_LANGS = new Set(['ar', 'dv', 'fa', 'he', 'ks', 'ku', 'ps', 'ur', 'ug', 'yi']);
const RTL_SCRIPTS = new Set(['Arab', 'Hebr', 'Thaa', 'Nkoo', 'Syrc']);

export function getBaseLanguage(locale: string): string {
    const normalized = normalizeLocaleTag(locale);
    if (!normalized) return '';
    return normalized.split('-')[0] ?? '';
}

export function normalizeLocaleTag(tag: string): string {
    const trimmed = tag.trim().replace(/_/g, '-');
    if (!trimmed) return '';
    const parts = trimmed.split('-');
    if (parts.length === 0) return '';
    const normalized = parts.map((part, index) => {
        if (!part) return '';
        if (index === 0) return part.toLowerCase();
        if (part.length === 2) return part.toUpperCase();
        if (part.length === 4) return part[0].toUpperCase() + part.slice(1).toLowerCase();
        return part.toLowerCase();
    });
    return normalized.every(Boolean) ? normalized.join('-') : '';
}

export function getLocaleDirection(locale: string): TextDirection {
    const normalized = normalizeLocaleTag(locale);
    if (!normalized) return 'ltr';
    const parts = normalized.split('-');
    const language = parts[0];
    if (RTL_LANGS.has(language)) return 'rtl';
    for (const part of parts) {
        if (RTL_SCRIPTS.has(part)) return 'rtl';
    }
    return 'ltr';
}
