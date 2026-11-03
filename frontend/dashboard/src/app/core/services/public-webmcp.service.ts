import { DOCUMENT } from '@angular/common';
import { effect, inject, Service, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { TranslocoService } from '@jsverse/transloco';
import { injectActiveLang } from '@core/i18n/active-lang';
import { browserBasePath } from '@core/interceptors/base-path.interceptor';
import { AnalyticsService } from './analytics.service';
import { AuthService } from './auth.service';
import type { DashboardAgentTool } from './dashboard-webmcp.service';

interface PublicModelContext {
    registerTool(tool: DashboardAgentTool, options: { signal: AbortSignal }): void | Promise<void>;
}

@Service()
export class PublicWebMCPService {
    private readonly document = inject(DOCUMENT);
    private readonly analytics = inject(AnalyticsService);
    private readonly auth = inject(AuthService);
    private readonly transloco = inject(TranslocoService);
    private readonly language = injectActiveLang();
    private readonly aiEnabled = signal(false);

    constructor() {
        const view = this.document.defaultView;
        const model = (this.document as Document & { modelContext?: PublicModelContext }).modelContext ?? (view?.navigator as (Navigator & { modelContext?: PublicModelContext }) | undefined)?.modelContext;
        if (!view?.isSecureContext || !model || !this.publicPage()) return;
        this.analytics
            .getSystemStatus()
            .pipe(takeUntilDestroyed())
            .subscribe({ next: (status) => this.aiEnabled.set(status.ai_enabled === true), error: () => this.aiEnabled.set(false) });
        effect((cleanup) => {
            this.language();
            if (!this.aiEnabled() || this.auth.isAuthenticated()) return;
            const controller = new AbortController();
            cleanup(() => controller.abort());
            const current = () => !controller.signal.aborted && this.aiEnabled() && !this.auth.isAuthenticated() && this.publicPage();
            try {
                void Promise.resolve(
                    model.registerTool(
                        {
                            name: 'hitkeep.public_connection_help',
                            title: this.transloco.translate('agentTools.publicTitle'),
                            description: this.transloco.translate('agentTools.publicDescription'),
                            inputSchema: { type: 'object', properties: {}, additionalProperties: false },
                            annotations: { readOnlyHint: true },
                            execute: async () => {
                                if (!current()) return { isError: true, content: [{ type: 'text', text: this.transloco.translate('agentTools.unavailable') }] };
                                const base = browserBasePath(this.document);
                                const help = {
                                    authentication_required: true,
                                    login_url: `${base}login`,
                                    authentication_url: `${base}auth.md`,
                                    api_contract_url: `${base}openapi.json`,
                                    documentation_url: 'https://hitkeep.com/guides/integrations/mcp/'
                                };
                                return { content: [{ type: 'text', text: JSON.stringify(help) }] };
                            }
                        },
                        { signal: controller.signal }
                    )
                ).catch(() => controller.abort());
            } catch {
                controller.abort();
            }
        });
    }

    private publicPage(): boolean {
        const view = this.document.defaultView;
        const embed = `${browserBasePath(this.document)}embed`;
        return !!view && view.top === view && view.location.pathname !== embed && !view.location.pathname.startsWith(`${embed}/`);
    }
}
