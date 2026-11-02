import { DOCUMENT } from '@angular/common';
import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { effect, inject, Service } from '@angular/core';
import { TranslocoService } from '@jsverse/transloco';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { firstValueFrom, fromEvent, takeUntil } from 'rxjs';
import { agentOperations, AgentOperation, agentRequest, redactAgentResult } from '@core/agent-tools/api-operations';
import { dialogCancelButton, dialogPrimaryButton } from '@components/dialog-actions/dialog-actions';
import { MainLayoutContextService } from '@layout/main-layout-context.service';
import { injectActiveLang } from '@core/i18n/active-lang';
import { AccessService } from './access.service';
import { AuthService } from './auth.service';
import { DashboardBootstrapService } from './dashboard-bootstrap.service';

export interface DashboardAgentTool {
    name: string;
    title: string;
    description: string;
    inputSchema: object;
    annotations: { readOnlyHint?: boolean; consequentialHint?: boolean; untrustedContentHint?: boolean };
    execute: (input: unknown, options?: { signal?: AbortSignal }) => Promise<object>;
}

interface ModelContext {
    registerTool(tool: DashboardAgentTool, options: { signal: AbortSignal }): Promise<void>;
}

function result(value: unknown): object {
    return { content: [{ type: 'text', text: JSON.stringify(value) }] };
}

@Service({ autoProvided: false })
export class DashboardWebMCPService {
    private readonly document = inject(DOCUMENT);
    private readonly http = inject(HttpClient);
    private readonly auth = inject(AuthService);
    private readonly bootstrap = inject(DashboardBootstrapService);
    private readonly context = inject(MainLayoutContextService);
    private readonly access = inject(AccessService);
    private readonly confirmation = inject(ConfirmationService);
    private readonly transloco = inject(TranslocoService);
    private readonly language = injectActiveLang();
    private operations: AgentOperation[] | null = null;
    private confirming = false;

    constructor() {
        effect((cleanup) => {
            this.language();
            const team = this.context.teamService.activeTeamId();
            const site = this.context.siteService.activeSite()?.id;
            const permissions = this.access.context();
            const model = (this.document as Document & { modelContext?: ModelContext }).modelContext;
            if (!this.available() || !model || !this.document.defaultView?.isSecureContext) return;
            const controller = new AbortController();
            cleanup(() => controller.abort());
            const current = () => !controller.signal.aborted && this.available() && team === this.context.teamService.activeTeamId() && site === this.context.siteService.activeSite()?.id && permissions === this.access.context();
            void this.register(model, controller, current).catch(() => controller.abort());
        });
    }

    private available(): boolean {
        return this.bootstrap.status()?.ai_enabled === true && this.auth.isAuthenticated() && !this.context.shareService.isShareMode() && !this.context.isContextPending();
    }

    private async register(model: ModelContext, controller: AbortController, current: () => boolean): Promise<void> {
        if (!this.operations) {
            const schema = await firstValueFrom(this.http.get<unknown>('/openapi.json').pipe(takeUntil(fromEvent(controller.signal, 'abort'))));
            this.operations = agentOperations(schema);
        }
        if (!current()) return;
        const operations = this.operations;
        await Promise.all([
            model.registerTool(
                {
                    name: 'hitkeep.list_actions',
                    title: this.transloco.translate('agentTools.listTitle'),
                    description: this.transloco.translate('agentTools.listDescription'),
                    inputSchema: { type: 'object', properties: { tag: { type: 'string' }, operation: { type: 'string' } }, additionalProperties: false },
                    annotations: { readOnlyHint: true },
                    execute: async (input) => {
                        if (!current()) return this.failure('agentTools.unavailable');
                        const filter = input && typeof input === 'object' ? (input as { tag?: string; operation?: string }) : {};
                        return result({
                            openapi_url: '/openapi.json',
                            team_id: this.context.teamService.activeTeamId(),
                            site_id: this.context.siteService.activeSite()?.id,
                            operations: operations.filter((operation) => (!filter.tag || operation.tags.includes(filter.tag)) && (!filter.operation || operation.id === filter.operation))
                        });
                    }
                },
                { signal: controller.signal }
            ),
            model.registerTool(
                {
                    name: 'hitkeep.run_action',
                    title: this.transloco.translate('agentTools.runTitle'),
                    description: this.transloco.translate('agentTools.runDescription'),
                    inputSchema: {
                        type: 'object',
                        properties: {
                            operation: { type: 'string' },
                            path_parameters: { type: 'object', additionalProperties: { type: 'string' } },
                            query: { type: 'object', additionalProperties: { type: ['string', 'number', 'boolean'] } },
                            body: {}
                        },
                        required: ['operation'],
                        additionalProperties: false
                    },
                    annotations: { consequentialHint: true, untrustedContentHint: true },
                    execute: (input, options) => this.run(operations, input, AbortSignal.any([controller.signal, ...(options?.signal ? [options.signal] : [])]), current)
                },
                { signal: controller.signal }
            )
        ]);
    }

    private async run(operations: AgentOperation[], input: unknown, signal: AbortSignal, current: () => boolean): Promise<object> {
        let sentMutation = false;
        try {
            if (!current() || signal.aborted) throw new Error('agentTools.unavailable');
            const id = input && typeof input === 'object' ? (input as { operation?: unknown }).operation : null;
            const operation = operations.find((operation) => operation.id === id);
            if (!operation) throw new Error('agentTools.invalidAction');
            const request = agentRequest(operation, input);
            if (operation.method !== 'GET') {
                await this.approve(operation, request, signal);
            }
            if (!current() || signal.aborted) throw new Error('agentTools.unavailable');
            sentMutation = operation.method !== 'GET';
            const response = await firstValueFrom(
                this.http
                    .request<unknown>(request.method, request.url, {
                        body: request.body,
                        observe: 'response',
                        headers: { 'X-HitKeep-Agent': 'webmcp', 'X-HitKeep-Agent-Team': this.context.teamService.activeTeamId() }
                    })
                    .pipe(takeUntil(fromEvent(signal, 'abort')))
            );
            if (!current() || signal.aborted) throw new Error('agentTools.unavailable');
            return result({ status: response.status, data: redactAgentResult(response.body) });
        } catch (error) {
            if (sentMutation && (!(error instanceof HttpErrorResponse) || error.status === 0 || error.status >= 500)) return this.failure('agentTools.outcomeUnknown');
            if (error instanceof HttpErrorResponse) return this.failure('agentTools.requestFailed', { status: error.status });
            const key = error instanceof Error && error.message.startsWith('agentTools.') ? error.message : 'agentTools.invalidAction';
            return this.failure(key);
        }
    }

    private approve(operation: AgentOperation, request: { url: string; body?: unknown }, signal: AbortSignal): Promise<void> {
        if (this.confirming) return Promise.reject(new Error('agentTools.confirmPending'));
        this.confirming = true;
        return new Promise<void>((resolve, reject) => {
            const finish = (accepted: boolean) => {
                signal.removeEventListener('abort', abort);
                this.confirming = false;
                if (accepted && !signal.aborted) resolve();
                else reject(new Error('agentTools.notConfirmed'));
            };
            const abort = () => {
                this.confirmation.close();
                finish(false);
            };
            signal.addEventListener('abort', abort, { once: true });
            this.confirmation.confirm({
                key: 'agent-action',
                header: this.transloco.translate('agentTools.confirmTitle'),
                message: this.transloco.translate('agentTools.confirmAction', {
                    action: `${operation.summary}\n${operation.method} ${request.url}`,
                    team: this.context.teamService.activeTeam()?.name ?? this.context.teamService.activeTeamId(),
                    details: request.body === undefined ? '' : JSON.stringify(request.body, null, 2)
                }),
                rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
                acceptButtonProps: dialogPrimaryButton(this.transloco.translate('common.actions.apply')),
                accept: () => finish(true),
                reject: () => finish(false)
            });
        });
    }

    private failure(key: string, params: Record<string, unknown> = {}): object {
        return { isError: true, content: [{ type: 'text', text: this.transloco.translate(key, params) }] };
    }
}
