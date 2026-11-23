import { Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Router, provideRouter, withComponentInputBinding } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { userEvent } from 'vitest/browser';
import { routes } from '../../app.routes';
import { INSTANCE_CAPABILITIES, INSTANCE_ROLE_CAPABILITIES } from '@core/access/capabilities';
import { MainLayoutContextService } from '@layout/main-layout-context.service';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { PermissionService } from '@services/permission.service';
import en from '../../../../public/i18n/en.json';

@Component({ template: '' })
class DashboardDestination {}

describe('global filter navigation', () => {
    let http: HttpTestingController;

    beforeEach(() => {
        const admin = routes.find((route) => route.path === '')!.children!.find((route) => route.path === 'admin')!;
        TestBed.configureTestingModule({
            imports: [TranslocoTestingModule.forRoot({ langs: { en }, translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }, preloadLangs: true })],
            providers: [
                provideRouter([admin, { path: 'dashboard', component: DashboardDestination }], withComponentInputBinding()),
                MainLayoutContextService,
                provideHttpClient(),
                provideHttpClientTesting(),
                provideTranslocoLocale({ langToLocaleMapping: { en: 'en-US' } })
            ]
        });
        http = TestBed.inject(HttpTestingController);
    });

    afterEach(() => http.verify());

    it.each([false, true])('routes the instance admin to filters and keeps system administration guarded (cloud %s)', async (hosted) => {
        TestBed.inject(DashboardBootstrapService).status.set({ needs_setup: false, version: 'test', cloud: { hosted, signup_enabled: hosted } });
        TestBed.inject(PermissionService).applyPermissions({ instance_role: 'admin', permissions: {}, instance_capabilities: [...INSTANCE_ROLE_CAPABILITIES.admin] });
        const harness = await RouterTestingHarness.create('/admin/exclusions');
        http.expectOne({ method: 'GET', url: '/api/admin/exclusions' }).flush([]);
        http.expectOne('/api/user/current-ip').flush({ cidr: '192.0.2.72/32' });
        harness.detectChanges();
        expect(harness.routeNativeElement?.textContent).toContain('Global traffic filters');
        await harness.navigateByUrl('/admin/system', DashboardDestination);
        expect(TestBed.inject(Router).url).toBe('/dashboard');
    });

    it.each([false, true])('denies a team and site owner with no instance authority (cloud %s)', async (hosted) => {
        TestBed.inject(DashboardBootstrapService).status.set({ needs_setup: false, version: 'test', cloud: { hosted, signup_enabled: hosted } });
        TestBed.inject(PermissionService).applyPermissions({ instance_role: 'user', active_team_role: 'owner', permissions: { 'site-1': 'owner' }, instance_capabilities: [] });
        await RouterTestingHarness.create('/admin/exclusions');
        expect(TestBed.inject(Router).url).toBe('/dashboard');
        http.expectNone('/api/admin/exclusions');
    });

    it('links System settings to the dedicated editor only when the user also manages filters', async () => {
        const permissions = TestBed.inject(PermissionService);
        permissions.applyPermissions({ instance_role: 'user', permissions: {}, instance_capabilities: [INSTANCE_CAPABILITIES.manageUsers] });
        const harness = await RouterTestingHarness.create('/admin/system');
        for (const list of ['users', 'sites', 'teams']) http.expectOne('/api/admin/' + list).flush([]);
        http.expectOne('/api/user/profile').flush({ id: 'operator', email: 'operator@example.test' });
        harness.detectChanges();
        const link = () => harness.routeNativeElement?.querySelector<HTMLAnchorElement>('a[href="/admin/exclusions"]');
        expect(link()).toBeNull();
        permissions.applyPermissions({ instance_role: 'owner', permissions: {}, instance_capabilities: [...INSTANCE_ROLE_CAPABILITIES.owner] });
        harness.detectChanges();
        expect(link()?.textContent).toContain('Global filters');
        link()!.focus();
        await userEvent.keyboard('{Enter}');
        await expect.poll(() => TestBed.inject(Router).url).toBe('/admin/exclusions');
        http.expectOne('/api/user/current-ip').flush({ cidr: '192.0.2.72/32' });
        http.expectOne('/api/admin/exclusions').flush([]);
    });

    it('lets an exclusion-only manager review and cancel a suggested IP by keyboard before explicitly submitting', async () => {
        TestBed.inject(PermissionService).applyPermissions({ instance_role: 'user', permissions: {}, instance_capabilities: [INSTANCE_CAPABILITIES.manageSiteExclusions] });
        const harness = await RouterTestingHarness.create('/admin/exclusions');
        http.expectOne('/api/user/current-ip').flush({ cidr: '192.0.2.72/32' });
        http.expectOne('/api/admin/exclusions').flush([]);
        harness.detectChanges();

        const suggestion = harness.routeNativeElement!.querySelector<HTMLButtonElement>('button[aria-label*="192.0.2.72/32"]')!;
        suggestion.focus();
        await userEvent.keyboard('{Enter}');
        const dialog = () => document.querySelector<HTMLElement>('[role="dialog"]');
        expect(dialog()?.querySelector<HTMLInputElement>('#instance-exclusion-cidr')?.value).toBe('192.0.2.72/32');
        await userEvent.keyboard('{Escape}');
        await expect.poll(() => dialog()).toBeNull();
        http.expectNone({ method: 'POST', url: '/api/admin/exclusions' });

        suggestion.focus();
        await userEvent.keyboard('{Enter}');
        const submit = Array.from(dialog()!.querySelectorAll('button')).find((button) => button.textContent?.includes('Add global filter'))!;
        submit.focus();
        await userEvent.keyboard('{Enter}');
        const create = http.expectOne({ method: 'POST', url: '/api/admin/exclusions' });
        expect(create.request.body).toMatchObject({ type: 'cidr', cidr: '192.0.2.72/32' });
        create.flush({ id: 'rule', type: 'cidr', cidr: '192.0.2.72/32', created_at: '2026-01-01T00:00:00Z' });
        harness.detectChanges();
        expect(harness.routeNativeElement?.textContent).toContain('Created global filter 192.0.2.72/32');
    });
});
