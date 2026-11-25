import { ChangeDetectionStrategy, Component, computed, input, linkedSignal } from '@angular/core';
import { NgOptimizedImage } from '@angular/common';
import { injectFaviconUrl } from '@core/analytics/favicon';

export interface SiteFaviconSource {
    domain: string;
}

@Component({
    selector: 'app-site-favicon',
    standalone: true,
    imports: [NgOptimizedImage],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        @for (image of visibleImages(); track image.url) {
            <img [ngSrc]="image.url" class="hk-site-favicon" [width]="20" [height]="20" loading="lazy" alt="" aria-hidden="true" (error)="showFallback(image.url)" />
        } @empty {
            <span class="hk-site-favicon-fallback" aria-hidden="true">
                <i class="pi pi-globe"></i>
            </span>
        }
    `,
    styles: [
        `
            :host {
                display: inline-flex;
                width: 1.25rem;
                height: 1.25rem;
                flex: none;
            }

            :host([compact]) {
                width: 1rem;
                height: 1rem;
            }

            .hk-site-favicon,
            .hk-site-favicon-fallback {
                width: 100%;
                height: 100%;
                flex: none;
                border-radius: 999px;
            }

            .hk-site-favicon {
                object-fit: cover;
            }

            .hk-site-favicon-fallback {
                display: inline-flex;
                align-items: center;
                justify-content: center;
                border: 1px solid var(--p-content-border-color);
                background: color-mix(in srgb, var(--p-primary-color) 10%, var(--p-content-background));
                color: var(--p-primary-color);
                line-height: 1;
            }

            :host-context(.p-dark) .hk-site-favicon-fallback {
                border-color: color-mix(in srgb, var(--p-content-border-color) 80%, var(--p-primary-color));
                background: color-mix(in srgb, var(--p-primary-color) 16%, var(--p-content-background));
            }

            .hk-site-favicon-fallback i {
                font-size: 0.7rem;
            }
        `
    ]
})
export class SiteFavicon {
    private readonly faviconUrlForDomain = injectFaviconUrl();
    site = input.required<SiteFaviconSource | null>();
    private readonly faviconUrl = computed(() => this.faviconUrlForDomain(this.site()?.domain));
    // Key the image node by URL so a late error cannot hide its replacement.
    protected readonly visibleImages = linkedSignal(() => {
        const url = this.faviconUrl();
        return url ? [{ url }] : [];
    });

    protected showFallback(url: string): void {
        if (url === this.faviconUrl()) {
            this.visibleImages.set([]);
        }
    }
}
