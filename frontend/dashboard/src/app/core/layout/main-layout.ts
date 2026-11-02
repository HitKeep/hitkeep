import { afterNextRender, ChangeDetectionStrategy, Component, computed, DestroyRef, ElementRef, inject, signal, ViewEncapsulation } from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { RouterOutlet } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { MessageModule } from '@openng/optimus-ui/message';
import { SplitterModule, SplitterResizeEndEvent } from '@openng/optimus-ui/splitter';
import { FreePlanRetentionNotice } from '@layout/free-plan-retention-notice';
import { LayoutOverlays } from '@layout/layout-overlays';
import { LayoutPageBar } from '@layout/layout-page-bar';
import { LayoutSidebar } from '@layout/layout-sidebar';
import { MainLayoutContextService } from '@layout/main-layout-context.service';
import { SidebarMenuService } from '@layout/sidebar-menu.service';
import { DashboardTitleService } from '@services/dashboard-title.service';
import { NavigationNoticeService } from '@services/navigation-notice.service';
import { DashboardWebMCPService } from '@services/dashboard-webmcp.service';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { ConfirmDialogModule } from '@openng/optimus-ui/confirmdialog';

@Component({
    selector: 'app-main-layout',
    encapsulation: ViewEncapsulation.None,
    changeDetection: ChangeDetectionStrategy.OnPush,
    providers: [MainLayoutContextService, SidebarMenuService, ConfirmationService, DashboardWebMCPService],
    imports: [NgTemplateOutlet, RouterOutlet, SplitterModule, LayoutSidebar, LayoutPageBar, LayoutOverlays, FreePlanRetentionNotice, MessageModule, TranslocoPipe, ConfirmDialogModule],
    templateUrl: './main-layout.html',
    styleUrl: './main-layout.css'
})
export class MainLayout {
    protected readonly context = inject(MainLayoutContextService);
    protected readonly dashboardTitle = inject(DashboardTitleService);
    protected readonly navigationNotice = inject(NavigationNoticeService);
    private readonly element = inject<ElementRef<HTMLElement>>(ElementRef);
    private readonly destroyRef = inject(DestroyRef);
    private readonly containerWidth = signal(window.innerWidth);
    protected readonly desktop = computed(() => this.containerWidth() > 768);
    protected readonly sidebarHidden = computed(() => !this.desktop() || this.context.isSidebarCollapsed());
    protected readonly panelSizes = computed(() => {
        const first = ((this.context.sidebarWidthPx() + 4) / this.containerWidth()) * 100;
        return [first, 100 - first];
    });
    protected readonly minSizes = computed(() => [(224 / this.containerWidth()) * 100, 100 - (324 / this.containerWidth()) * 100]);

    constructor() {
        inject(DashboardWebMCPService);
        afterNextRender(() => {
            const observer = new ResizeObserver(([entry]) => {
                const width = Math.max(1, entry.contentRect.width);
                this.containerWidth.set(width);
                if (width > 768) this.context.isMobileDrawerOpen.set(false);
            });
            observer.observe(this.element.nativeElement);
            this.destroyRef.onDestroy(() => observer.disconnect());
        });
    }

    onSidebarResize(event: SplitterResizeEndEvent): void {
        if (!this.sidebarHidden()) this.context.setSidebarWidthPx((Number(event.sizes[0]) * this.containerWidth()) / 100 - 4);
    }
}
