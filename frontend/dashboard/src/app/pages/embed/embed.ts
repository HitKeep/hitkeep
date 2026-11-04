import { HttpClient } from '@angular/common/http';
import { ChangeDetectionStrategy, Component, DestroyRef, effect, inject, signal, untracked } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ActivatedRoute, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { combineLatest } from 'rxjs';
import { SiteService } from '@features/sites/services/site.service';
import { Site } from '@models/analytics.types';
import { EmbedService } from '@services/embed.service';
import { ShareService } from '@services/share.service';

export const EMBED_ANALYTICS_TABS = [
    { path: 'dashboard', key: 'nav.dashboard' },
    { path: 'events', key: 'nav.events' },
    { path: 'goals', key: 'nav.goals' },
    { path: 'funnels', key: 'nav.funnels' },
    { path: 'ecommerce', key: 'nav.ecommerce' },
    { path: 'web-vitals', key: 'nav.webVitals' },
    { path: 'ai-agents', key: 'nav.aiAgents' },
    { path: 'ai-chatbots', key: 'nav.aiChatbots' },
    { path: 'utm', key: 'nav.utm' }
];

@Component({
    selector: 'app-embed-dashboard',
    imports: [RouterOutlet, RouterLink, RouterLinkActive, TranslocoPipe, ButtonModule],
    template: `
        <main class="min-h-dvh bg-[var(--p-surface-ground)] p-3 text-color sm:p-5" [attr.aria-label]="'nav.contentAreaAria' | transloco">
            <header class="mb-4 flex flex-wrap items-center justify-between gap-2 border-b border-surface-200 pb-3 dark:border-surface-700">
                <h1 class="m-0 min-w-0 break-all text-lg font-semibold">{{ sites.activeSite()?.domain || 'HitKeep' }}</h1>
                <span class="text-sm text-muted-color">{{ 'embed.readOnly' | transloco }}</span>
            </header>
            @if (embed.status() === 'error' || loadError()) {
                <div class="py-12 text-center" role="alert">
                    <p class="mb-4">{{ 'embed.connectionError' | transloco }}</p>
                    <p-button [label]="'oauth.retry' | transloco" (onClick)="retry()" />
                </div>
            } @else if (!sites.activeSite()) {
                <p class="py-12 text-center text-muted-color" role="status">{{ 'common.loadingSiteData' | transloco }}</p>
            } @else {
                <nav class="mb-5 flex gap-1 overflow-x-auto border-b border-surface-200 pb-2 dark:border-surface-700" [attr.aria-label]="'embed.analyticsNavigation' | transloco">
                    @for (tab of tabs; track tab.path) {
                        <a
                            [routerLink]="[tab.path]"
                            queryParamsHandling="preserve"
                            routerLinkActive="bg-highlight text-primary"
                            ariaCurrentWhenActive="page"
                            class="shrink-0 rounded px-3 py-2 text-sm font-medium hover:bg-emphasis focus-visible:outline-2"
                            >{{ tab.key | transloco }}</a
                        >
                    }
                </nav>
                <router-outlet />
            }
        </main>
    `,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class EmbedDashboard {
    protected readonly sites = inject(SiteService);
    protected readonly embed = inject(EmbedService);
    protected readonly tabs = EMBED_ANALYTICS_TABS;
    protected readonly loadError = signal(false);
    private readonly http = inject(HttpClient);
    private readonly route = inject(ActivatedRoute);

    constructor() {
        inject(ShareService).clear();
        this.sites.sites.set([]);
        this.sites.activeSite.set(null);
        let currentLaunch = '';
        combineLatest([this.route.paramMap, this.route.queryParamMap])
            .pipe(takeUntilDestroyed())
            .subscribe(([params, query]) => {
                const siteId = params.get('siteId') ?? '';
                const launch = query.get('launch') ?? '';
                const key = `${siteId}:${launch}`;
                if (key === currentLaunch) return;
                currentLaunch = key;
                this.sites.sites.set([]);
                this.sites.activeSite.set(null);
                this.embed.start(siteId, launch);
            });
        effect((onCleanup) => {
            const credential = this.embed.credential();
            if (!credential) {
                this.sites.sites.set([]);
                this.sites.activeSite.set(null);
                return;
            }
            if (untracked(() => this.sites.activeSite()?.id) === credential.siteId) return;
            this.loadError.set(false);
            const subscription = this.http.get<Site>(`/api/embed/sites/${encodeURIComponent(credential.siteId)}/site`).subscribe({
                next: (site) => {
                    if (site.id !== credential.siteId) {
                        this.embed.fail();
                        return;
                    }
                    this.sites.sites.set([site]);
                    this.sites.activeSite.set(site);
                },
                error: () => this.loadError.set(true)
            });
            onCleanup(() => subscription.unsubscribe());
        });
        inject(DestroyRef).onDestroy(() => {
            this.embed.stop();
            this.sites.sites.set([]);
            this.sites.activeSite.set(null);
        });
    }

    protected retry(): void {
        this.loadError.set(false);
        this.embed.fail();
        this.embed.requestRefresh();
    }
}
