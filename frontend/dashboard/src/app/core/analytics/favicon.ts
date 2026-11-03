import { DOCUMENT } from '@angular/common';
import { inject } from '@angular/core';
import { browserAppUrl } from '@core/interceptors/base-path.interceptor';
import { EmbedService } from '@services/embed.service';

export function injectFaviconUrl(): (domain: string | null | undefined) => string | null {
    const document = inject(DOCUMENT);
    const embed = inject(EmbedService);
    // Image requests bypass HttpClient and would send dashboard cookies in an embed.
    return (domain) => (domain && !embed.isEmbedMode() ? browserAppUrl(document, `/api/favicon/${encodeURIComponent(domain)}`) : null);
}
