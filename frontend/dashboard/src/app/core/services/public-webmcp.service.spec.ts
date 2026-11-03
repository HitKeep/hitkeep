import { Component, inject, signal } from '@angular/core';
import { DOCUMENT } from '@angular/common';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { Subject } from 'rxjs';
import { vi } from 'vitest';
import { PublicWebMCPService } from './public-webmcp.service';
import { AnalyticsService } from './analytics.service';
import { AuthService } from './auth.service';
import type { DashboardAgentTool } from './dashboard-webmcp.service';

let publicDocument: Document;

@Component({ template: '', providers: [PublicWebMCPService, { provide: DOCUMENT, useFactory: () => publicDocument }] })
class Probe {
    readonly tools = inject(PublicWebMCPService);
}

describe('PublicWebMCPService', () => {
    let fixture: ComponentFixture<Probe>;
    let status: Subject<{ ai_enabled: boolean }>;
    let tools: Map<string, DashboardAgentTool>;
    let model: { registerTool: ReturnType<typeof vi.fn> };
    let originalDocument: PropertyDescriptor | undefined;
    let originalNavigator: PropertyDescriptor | undefined;
    let originalSecure: PropertyDescriptor | undefined;
    let topLevel: boolean;
    let pathname: string;
    const authenticated = signal(false);
    const analytics = { getSystemStatus: vi.fn() };

    beforeEach(async () => {
        status = new Subject();
        authenticated.set(false);
        topLevel = true;
        pathname = '/login';
        const view: Window = new Proxy(window, {
            get(target, key) {
                if (key === 'top') return topLevel ? view : {};
                if (key === 'location') return { pathname };
                return Reflect.get(target, key, target);
            }
        });
        publicDocument = new Proxy(document, {
            get(target, key) {
                if (key === 'defaultView') return view;
                const value = Reflect.get(target, key, target);
                return typeof value === 'function' ? value.bind(target) : value;
            }
        });
        tools = new Map();
        analytics.getSystemStatus.mockReset().mockReturnValue(status.asObservable());
        originalDocument = Object.getOwnPropertyDescriptor(document, 'modelContext');
        originalNavigator = Object.getOwnPropertyDescriptor(navigator, 'modelContext');
        originalSecure = Object.getOwnPropertyDescriptor(window, 'isSecureContext');
        model = {
            registerTool: vi.fn((tool: DashboardAgentTool, options: { signal: AbortSignal }) => {
                tools.set(tool.name, tool);
                options.signal.addEventListener('abort', () => tools.delete(tool.name), { once: true });
            })
        };
        Object.defineProperty(document, 'modelContext', { configurable: true, value: model });
        Object.defineProperty(window, 'isSecureContext', { configurable: true, value: true });
        await TestBed.configureTestingModule({
            imports: [Probe, TranslocoTestingModule.forRoot({ langs: { en: { agentTools: { publicTitle: 'Connection help', publicDescription: 'Public guidance only.' } } }, translocoConfig: { defaultLang: 'en', availableLangs: ['en'] } })],
            providers: [
                { provide: AnalyticsService, useValue: analytics },
                { provide: AuthService, useValue: { isAuthenticated: authenticated } }
            ]
        }).compileComponents();
    });

    afterEach(() => {
        fixture?.destroy();
        for (const [target, key, original] of [
            [document, 'modelContext', originalDocument],
            [navigator, 'modelContext', originalNavigator],
            [window, 'isSecureContext', originalSecure]
        ] as const) {
            if (original) Object.defineProperty(target, key, original);
            else Reflect.deleteProperty(target, key);
        }
    });

    async function start(enabled = true) {
        fixture = TestBed.createComponent(Probe);
        status.next({ ai_enabled: enabled });
        await fixture.whenStable();
    }

    it('registers only genuine public help before sign-in, without fetching analytics or API action schemas', async () => {
        await start();
        expect(analytics.getSystemStatus).toHaveBeenCalledTimes(1);
        expect([...tools.keys()]).toEqual(['hitkeep.public_connection_help']);
        const tool = tools.get('hitkeep.public_connection_help')!;
        expect(tool.annotations).toEqual({ readOnlyHint: true });
        const output = JSON.stringify(await tool.execute({}));
        expect(output).toContain('authentication_required');
        expect(output).toContain('/auth.md');
        expect(output).not.toContain('team_id');
        expect(output).not.toContain('site_id');
        expect(tools.has('hitkeep.run_action')).toBe(false);
    });

    it('honors AI off and invalidates stale callbacks after flag disable or sign-in', async () => {
        await start(false);
        expect(tools.size).toBe(0);
        status.next({ ai_enabled: true });
        await fixture.whenStable();
        const tool = tools.get('hitkeep.public_connection_help')!;
        authenticated.set(true);
        expect(await tool.execute({})).toMatchObject({ isError: true });
        await fixture.whenStable();
        expect(tools.size).toBe(0);
        authenticated.set(false);
        await fixture.whenStable();
        const newTool = tools.get('hitkeep.public_connection_help')!;
        status.next({ ai_enabled: false });
        expect(await newTool.execute({})).toMatchObject({ isError: true });
        await fixture.whenStable();
        expect(tools.size).toBe(0);
    });

    it('skips insecure browsers without a status request', async () => {
        Object.defineProperty(window, 'isSecureContext', { configurable: true, value: false });
        await start();
        expect(analytics.getSystemStatus).not.toHaveBeenCalled();
        expect(tools.size).toBe(0);
    });

    it('does not request account APIs or register tools inside child frames', async () => {
        topLevel = false;
        await start();
        expect(analytics.getSystemStatus).not.toHaveBeenCalled();
        expect(tools.size).toBe(0);
    });

    it('does not request account APIs on an embed route opened at top level', async () => {
        pathname = '/embed/site-1';
        await start();
        expect(analytics.getSystemStatus).not.toHaveBeenCalled();
        expect(tools.size).toBe(0);
    });

    it('does not break the app when browser policy refuses registration', async () => {
        model.registerTool.mockImplementation(() => {
            throw new Error('NotAllowedError');
        });
        await start();
        expect(tools.size).toBe(0);
    });

    it('uses the deprecated navigator surface only when the native document surface is absent', async () => {
        Reflect.deleteProperty(document, 'modelContext');
        Object.defineProperty(navigator, 'modelContext', { configurable: true, value: model });
        await start();
        expect(tools.size).toBe(1);
    });

    it('does not replace the native browser surface with a polyfill', async () => {
        const fallback = { registerTool: vi.fn() };
        Object.defineProperty(navigator, 'modelContext', { configurable: true, value: fallback });
        await start();
        expect(model.registerTool).toHaveBeenCalledTimes(1);
        expect(fallback.registerTool).not.toHaveBeenCalled();
    });

    it('skips browsers without WebMCP without a status request', async () => {
        Reflect.deleteProperty(document, 'modelContext');
        Reflect.deleteProperty(navigator, 'modelContext');
        await start();
        expect(analytics.getSystemStatus).not.toHaveBeenCalled();
        expect(tools.size).toBe(0);
    });

    it('keeps public connection URLs within the application prefix', async () => {
        await start();
        const existing = document.querySelector('base');
        const base = existing ?? document.createElement('base');
        if (!existing) document.head.append(base);
        const original = base.getAttribute('href');
        base.setAttribute('href', '/tenant/');
        try {
            const output = JSON.stringify(await tools.get('hitkeep.public_connection_help')!.execute({}));
            expect(output).toContain('/tenant/auth.md');
            expect(output).toContain('/tenant/login');
        } finally {
            if (!existing) base.remove();
            else if (original === null) base.removeAttribute('href');
            else base.setAttribute('href', original);
        }
    });
});
