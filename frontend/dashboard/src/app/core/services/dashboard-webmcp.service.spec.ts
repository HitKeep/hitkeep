import { Component, inject, signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { vi } from 'vitest';
import { DashboardWebMCPService, DashboardAgentTool } from './dashboard-webmcp.service';
import { DashboardBootstrapService } from './dashboard-bootstrap.service';
import { AuthService } from './auth.service';
import { AccessService } from './access.service';
import { MainLayoutContextService } from '@layout/main-layout-context.service';

@Component({ template: '', providers: [DashboardWebMCPService] })
class Probe {
    readonly tools = inject(DashboardWebMCPService);
}

describe('DashboardWebMCPService', () => {
    let fixture: ComponentFixture<Probe>;
    let http: HttpTestingController;
    let tools: Map<string, DashboardAgentTool>;
    const status = signal({ ai_enabled: false });
    const authenticated = signal(true);
    const share = signal(false);
    const pending = signal(false);
    const team = signal('team-1');
    const site = signal({ id: 'site-1' });
    const permissions = signal({});
    const confirmation = { confirm: vi.fn(), close: vi.fn() };
    let original: PropertyDescriptor | undefined;

    beforeEach(async () => {
        tools = new Map();
        original = Object.getOwnPropertyDescriptor(document, 'modelContext');
        Object.defineProperty(document, 'modelContext', {
            configurable: true,
            value: {
                registerTool: vi.fn(async (tool: DashboardAgentTool, options: { signal: AbortSignal }) => {
                    tools.set(tool.name, tool);
                    options.signal.addEventListener('abort', () => tools.delete(tool.name), { once: true });
                })
            }
        });
        status.set({ ai_enabled: false });
        authenticated.set(true);
        share.set(false);
        pending.set(false);
        team.set('team-1');
        site.set({ id: 'site-1' });
        permissions.set({});
        confirmation.confirm.mockReset();
        confirmation.close.mockReset();
        await TestBed.configureTestingModule({
            imports: [
                Probe,
                TranslocoTestingModule.forRoot({
                    langs: { en: { agentTools: { confirmTitle: 'Confirm agent action', confirmAction: '{{action}} on {{team}}\n{{details}}' }, common: { actions: { apply: 'Apply', cancel: 'Cancel' } } } },
                    translocoConfig: { defaultLang: 'en', availableLangs: ['en'] }
                })
            ],
            providers: [
                provideHttpClient(),
                provideHttpClientTesting(),
                { provide: DashboardBootstrapService, useValue: { status } },
                { provide: AuthService, useValue: { isAuthenticated: authenticated } },
                { provide: AccessService, useValue: { context: permissions } },
                { provide: ConfirmationService, useValue: confirmation },
                {
                    provide: MainLayoutContextService,
                    useValue: { shareService: { isShareMode: share }, isContextPending: pending, teamService: { activeTeamId: team, activeTeam: () => ({ name: 'Team one' }) }, siteService: { activeSite: site } }
                }
            ]
        }).compileComponents();
        fixture = TestBed.createComponent(Probe);
        http = TestBed.inject(HttpTestingController);
        await fixture.whenStable();
    });

    afterEach(() => {
        fixture.destroy();
        http.verify();
        if (original) Object.defineProperty(document, 'modelContext', original);
        else Reflect.deleteProperty(document, 'modelContext');
    });

    async function enable() {
        status.set({ ai_enabled: true });
        await fixture.whenStable();
        http.expectOne('/openapi.json').flush({ paths: { '/api/sites/{id}/goals': { get: { summary: 'List goals', tags: ['Goals'] }, post: { summary: 'Create goal', tags: ['Goals'] } } } });
        await fixture.whenStable();
    }

    it('makes no schema request or registration while AI is disabled', () => {
        http.expectNone('/openapi.json');
        expect(tools.size).toBe(0);
    });

    it('gracefully skips browsers without WebMCP', async () => {
        Reflect.deleteProperty(document, 'modelContext');
        status.set({ ai_enabled: true });
        await fixture.whenStable();
        http.expectNone('/openapi.json');
        expect(tools.size).toBe(0);
    });

    it('lists actual API operations and uses the existing authenticated HTTP client for reads', async () => {
        await enable();
        const catalog = await tools.get('hitkeep.list_actions')!.execute({});
        expect(JSON.stringify(catalog)).toContain('GET /api/sites/{id}/goals');
        const result = tools.get('hitkeep.run_action')!.execute({ operation: 'GET /api/sites/{id}/goals', path_parameters: { id: 'site-1' } });
        const request = http.expectOne('/api/sites/site-1/goals');
        expect(request.request.headers.get('X-HitKeep-Agent')).toBe('webmcp');
        expect(request.request.headers.get('X-HitKeep-Agent-Team')).toBe('team-1');
        request.flush([{ name: 'Purchase' }]);
        expect(JSON.stringify(await result)).toContain('Purchase');
        expect(confirmation.confirm).not.toHaveBeenCalled();
    });

    it('waits for human confirmation before sending a mutation and redacts credentials in results', async () => {
        await enable();
        const result = tools.get('hitkeep.run_action')!.execute({ operation: 'POST /api/sites/{id}/goals', path_parameters: { id: 'site-1' }, body: { name: 'Purchase' } });
        await Promise.resolve();
        http.expectNone('/api/sites/site-1/goals');
        const dialog = confirmation.confirm.mock.calls[0][0];
        expect(dialog.message).toContain('Create goal');
        expect(dialog.message).toContain('Purchase');
        dialog.accept();
        await Promise.resolve();
        http.expectOne('/api/sites/site-1/goals').flush({ name: 'Purchase', token: 'never-show-this' });
        expect(JSON.stringify(await result)).not.toContain('never-show-this');
    });

    it('does not send a rejected mutation', async () => {
        await enable();
        const result = tools.get('hitkeep.run_action')!.execute({ operation: 'POST /api/sites/{id}/goals', path_parameters: { id: 'site-1' }, body: {} });
        await Promise.resolve();
        confirmation.confirm.mock.calls[0][0].reject();
        expect(await result).toMatchObject({ isError: true });
        http.expectNone('/api/sites/site-1/goals');
    });

    it('rejects stale callbacks immediately after logout or flag disable', async () => {
        await enable();
        const run = tools.get('hitkeep.run_action')!;
        authenticated.set(false);
        expect(await run.execute({ operation: 'GET /api/sites/{id}/goals', path_parameters: { id: 'site-1' } })).toMatchObject({ isError: true });
        await fixture.whenStable();
        expect(tools.size).toBe(0);
        http.expectNone('/api/sites/site-1/goals');
    });

    it('cancels approval when the active context changes', async () => {
        await enable();
        const run = tools.get('hitkeep.run_action')!;
        const result = run.execute({ operation: 'POST /api/sites/{id}/goals', path_parameters: { id: 'site-1' }, body: {} });
        await Promise.resolve();
        const dialog = confirmation.confirm.mock.calls[0][0];
        team.set('team-2');
        dialog.accept();
        expect(await result).toMatchObject({ isError: true });
        http.expectNone('/api/sites/site-1/goals');
    });

    it('blocks unknown and traversal operations without sending HTTP', async () => {
        await enable();
        const run = tools.get('hitkeep.run_action')!;
        for (const input of [{ operation: 'GET https://external.example/' }, { operation: 'GET /api/sites/{id}/goals', path_parameters: { id: '../admin' } }]) {
            expect(await run.execute(input)).toMatchObject({ isError: true });
        }
        expect(confirmation.confirm).not.toHaveBeenCalled();
    });

    it('warns against retrying a mutation interrupted after it was sent', async () => {
        await enable();
        const execution = tools.get('hitkeep.run_action')!.execute({ operation: 'POST /api/sites/{id}/goals', path_parameters: { id: 'site-1' }, body: { name: 'Purchase' } });
        await Promise.resolve();
        confirmation.confirm.mock.calls[0][0].accept();
        await Promise.resolve();
        const request = http.expectOne('/api/sites/site-1/goals');
        request.error(new ProgressEvent('error'));
        expect(JSON.stringify(await execution)).toContain('agentTools.outcomeUnknown');
    });
});
