import { computed, inject, signal } from '@angular/core';
import { rxResource } from '@angular/core/rxjs-interop';
import type { Observable } from 'rxjs';
import { SiteService } from '@features/sites/services/site.service';

export function injectConversionDefinitions<T>(load: (siteId: string) => Observable<T[]>) {
    const sites = inject(SiteService);
    const siteId = computed(() => sites.activeSite()?.id);
    const revision = signal(0);
    const definitions = rxResource({
        params: () => {
            const id = siteId();
            return id ? { id, revision: revision() } : undefined;
        },
        stream: ({ params }) => load(params.id)
    });
    return {
        items: computed(() => (definitions.hasValue() ? (definitions.value() ?? []) : [])),
        loading: definitions.isLoading,
        error: computed(() => !!definitions.error()),
        // Mutations must supersede even an in-flight definitions read.
        reload: () => revision.update((value) => value + 1)
    };
}
