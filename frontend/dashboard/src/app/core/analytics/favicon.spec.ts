import { DOCUMENT } from '@angular/common';
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { EmbedService } from '@services/embed.service';
import { injectFaviconUrl } from './favicon';

describe('favicon request boundary', () => {
    it('preserves prefixed, encoded favicon URLs for ordinary views and suppresses them throughout embed mode', () => {
        const embedded = signal(false);
        TestBed.configureTestingModule({
            providers: [
                { provide: DOCUMENT, useValue: { querySelector: () => ({ getAttribute: () => '/hitkeep/' }) } },
                { provide: EmbedService, useValue: { isEmbedMode: embedded } }
            ]
        });
        const faviconUrl = TestBed.runInInjectionContext(injectFaviconUrl);
        expect(faviconUrl('example.com/path')).toBe('/hitkeep/api/favicon/example.com%2Fpath');
        expect(faviconUrl(null)).toBeNull();
        embedded.set(true);
        expect(faviconUrl('example.com')).toBeNull();
        embedded.set(false);
        expect(faviconUrl('example.com')).toBe('/hitkeep/api/favicon/example.com');
    });
});
