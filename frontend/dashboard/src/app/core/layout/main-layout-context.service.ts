import { TemplateRef, computed, inject, signal, Service } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router } from '@angular/router';
import { filter, map, startWith } from 'rxjs';
import { Site, Team } from '@models/analytics.types';
import { TEAM_CAPABILITIES } from '@core/access/capabilities';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { AccessService } from '@services/access.service';
import { PermissionService } from '@services/permission.service';
import { ShareService } from '@services/share.service';
import { TeamService } from '@services/team.service';
import type { SiteSettingsSection } from '@features/sites/site-settings-section';
import { SiteService } from '@features/sites/services/site.service';

@Service({ autoProvided: false })
export class MainLayoutContextService {
    private readonly router = inject(Router);
    readonly siteService = inject(SiteService);
    readonly shareService = inject(ShareService);
    private readonly bootstrap = inject(DashboardBootstrapService);
    private readonly access = inject(AccessService);
    readonly teamService = inject(TeamService);
    readonly perms = inject(PermissionService);

    readonly cloudHosted = this.bootstrap.cloudHosted;
    readonly cloudSupportUrl = this.bootstrap.cloudSupportUrl;
    // Self-hosted deployments have no plan limits; managed cloud trusts the
    // server-derived flag instead of re-deriving policy in the frontend.
    readonly canCreateTeams = computed(() => !this.cloudHosted() || this.perms.canCreateTeams());
    readonly isTeamAdmin = computed(() => this.access.canActiveTeam(TEAM_CAPABILITIES.manageSettings));

    readonly isMobileDrawerOpen = signal(false);
    private readonly sidebarPreference = this.readSidebarPreference();
    readonly isSidebarCollapsed = signal(this.sidebarPreference.collapsed);
    readonly sidebarWidthPx = signal(this.sidebarPreference.widthPx);
    readonly isAddSiteVisible = signal(false);
    readonly isCreateTeamVisible = signal(false);
    readonly pageHeaderLeft = signal<TemplateRef<unknown> | null>(null);
    readonly pageHeaderRight = signal<TemplateRef<unknown> | null>(null);
    readonly hasPageHeader = computed(() => this.pageHeaderLeft() !== null);
    // The overview page is team-scoped, so the site selector and its settings
    // shortcut do not apply there and the shell hides them.
    readonly isOverviewPage = computed(() => {
        const url = this.activeUrl().split(/[?#]/, 1)[0] || '/';
        return url === '/overview' || url.startsWith('/overview/');
    });

    readonly beforeTeamSwitch = () => true;
    private pageHeaderOwner: symbol | null = null;
    private readonly activeUrl = toSignal(
        this.router.events.pipe(
            filter((event): event is NavigationEnd => event instanceof NavigationEnd),
            map((event) => event.urlAfterRedirects),
            startWith(this.router.url)
        ),
        { initialValue: this.router.url }
    );

    toggleSidebar(): void {
        this.isSidebarCollapsed.update((collapsed) => !collapsed);
        this.storeSidebarPreference();
    }

    setSidebarWidthPx(widthPx: number): void {
        this.sidebarWidthPx.set(Number.isFinite(widthPx) ? Math.min(320, Math.max(220, Math.round(widthPx))) : 240);
        this.storeSidebarPreference();
    }

    private readSidebarPreference(): { collapsed: boolean; widthPx: number } {
        try {
            const value = JSON.parse(localStorage.getItem('hk_sidebar_layout') ?? 'null');
            if (value?.version === 1 && typeof value.widthPx === 'number' && Number.isFinite(value.widthPx) && typeof value.collapsed === 'boolean') {
                return { collapsed: value.collapsed, widthPx: Math.min(320, Math.max(220, Math.round(value.widthPx))) };
            }
        } catch {
            // Navigation remains usable when browser storage is unavailable.
        }
        return { collapsed: false, widthPx: 240 };
    }

    private storeSidebarPreference(): void {
        try {
            localStorage.setItem('hk_sidebar_layout', JSON.stringify({ version: 1, collapsed: this.isSidebarCollapsed(), widthPx: this.sidebarWidthPx() }));
        } catch {
            // Keep the in-memory preference when storage is blocked or full.
        }
    }

    openSiteSettings(section: SiteSettingsSection = 'general') {
        const site = this.siteService.activeSite();
        if (!site) return;
        void this.router.navigate(['/sites', site.id, 'settings', section]);
    }

    onSiteSelected(site: Site) {
        this.siteService.selectSite(site);
        const section = this.currentSiteSettingsSection();
        if (section) {
            void this.router.navigate(['/sites', site.id, 'settings', section]);
        }
    }

    onTeamSelected(team: Team) {
        void this.router.navigate(['/overview']);
        this.teamService.setActiveTeam(team.id).subscribe({
            next: () => {
                this.siteService.sites.set([]);
                this.siteService.activeSite.set(null);
                this.teamService.loadTeams().subscribe({
                    next: () => {
                        this.siteService.loadSites();
                        this.perms.loadPermissions().subscribe({
                            next: () => this.redirectIfTeamAdminAccessWasLost(),
                            error: () => this.redirectIfTeamAdminAccessWasLost()
                        });
                    },
                    error: () => {
                        this.siteService.loadSites();
                        this.perms.loadPermissions().subscribe({
                            next: () => this.redirectIfTeamAdminAccessWasLost(),
                            error: () => this.redirectIfTeamAdminAccessWasLost()
                        });
                    }
                });
            },
            error: () => undefined
        });
    }

    registerPageHeader(owner: symbol, left: TemplateRef<unknown> | null, right: TemplateRef<unknown> | null) {
        this.pageHeaderOwner = owner;
        this.pageHeaderLeft.set(left);
        this.pageHeaderRight.set(right);
    }

    clearPageHeader(owner: symbol) {
        if (this.pageHeaderOwner !== owner) {
            return;
        }

        this.pageHeaderOwner = null;
        this.pageHeaderLeft.set(null);
        this.pageHeaderRight.set(null);
    }

    private redirectIfTeamAdminAccessWasLost() {
        const currentURL = this.router.routerState.snapshot.url;
        if ((currentURL.startsWith('/admin/team') || currentURL.startsWith('/integration/google-search-console')) && !this.isTeamAdmin()) {
            this.router.navigateByUrl('/dashboard');
        }
    }

    private currentSiteSettingsSection(): SiteSettingsSection | null {
        const match = this.router.url.match(/^\/sites\/[^/]+\/settings\/(general|tracking|filtering|retention|access|share|danger-zone)(?:[/?#]|$)/);
        return (match?.[1] as SiteSettingsSection | undefined) ?? null;
    }
}
