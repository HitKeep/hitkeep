import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { Router } from '@angular/router';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';

import { SiteService } from '@features/sites/services/site.service';
import { AddSiteDialog } from './add-site-dialog';

describe('AddSiteDialog', () => {
    let fixture: ComponentFixture<AddSiteDialog>;
    const navigate = vi.fn();
    const navigateByUrl = vi.fn();
    const hosted = vi.fn(() => true);
    let createSite: ReturnType<typeof vi.fn>;

    beforeEach(async () => {
        createSite = vi.fn(() =>
            of({
                id: 'site-1',
                user_id: 'user-1',
                domain: 'sub.example-app.com.br',
                created_at: '2026-05-05T00:00:00Z'
            })
        );

        await TestBed.configureTestingModule({
            imports: [
                AddSiteDialog,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: { actions: { cancel: 'Cancel' } },
                            sites: {
                                addDialog: {
                                    title: 'Add site',
                                    addAction: 'Add site',
                                    instructionsLine1: 'Enter a domain such as {{apex}} or {{subdomain}}.',
                                    instructionsLine2: 'We automatically track {{www}}.',
                                    domainLabel: 'Domain',
                                    domainPlaceholder: 'example.com',
                                    errors: {
                                        domainRequired: 'Domain is required.',
                                        domainInvalid: 'Invalid domain format.',
                                        removeProtocol: 'Remove the protocol.',
                                        removeWww: 'Enter the domain without the www prefix.',
                                        createFailed: 'Failed to create site.'
                                    }
                                }
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [
                { provide: SiteService, useValue: { createSite, sites: () => [] } },
                { provide: Router, useValue: { navigate, navigateByUrl } },
                { provide: DashboardBootstrapService, useValue: { cloudHosted: hosted } }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(AddSiteDialog);
        fixture.componentRef.setInput('visible', true);
        await fixture.whenStable();
    });

    afterEach(() => {
        fixture.destroy();
        document.querySelectorAll('.p-dialog-mask, .p-dialog').forEach((element) => element.remove());
    });

    it('submits an issue-style hyphenated multi-level domain', async () => {
        const input = document.body.querySelector('#domain') as HTMLInputElement;
        input.value = 'sub.example-app.com.br';
        input.dispatchEvent(new Event('input'));
        input.dispatchEvent(new Event('blur'));
        await fixture.whenStable();

        const buttons = Array.from(document.body.querySelectorAll('.dialog-shell-footer button')) as HTMLButtonElement[];
        const addButton = buttons.at(-1);
        expect(addButton?.disabled).toBe(false);

        addButton?.click();
        await fixture.whenStable();

        expect(createSite).toHaveBeenCalledWith('sub.example-app.com.br');
        expect(navigate).toHaveBeenCalledWith(['/sites', 'site-1', 'settings', 'tracking']);
    });

    it.each([{ status: 402 }, { status: 403, error: { code: 'site_limit_reached' } }, { status: 403, error: 'Site limit reached\n' }])('opens the Cloud upgrade from a quota error', async (error) => {
        createSite.mockReturnValue(throwError(() => error));
        const component = fixture.componentInstance;
        component['form'].domain().control().setValue('test.example');
        component.onSubmit();
        await fixture.whenStable();
        const button = document.body.querySelector('p-message p-button button') as HTMLButtonElement;
        expect(button).toBeTruthy();
        button.click();
        expect(component.visible()).toBe(false);
        expect(navigateByUrl).toHaveBeenCalledWith('/admin/team/overview');
    });

    it('keeps quota errors free of billing actions outside Cloud', () => {
        hosted.mockReturnValueOnce(false);
        createSite.mockReturnValue(throwError(() => ({ status: 402 })));
        const component = fixture.componentInstance;
        component['form'].domain().control().setValue('test.example');
        component.onSubmit();
        expect(component['createError']()).toBe('sites.addDialog.errors.createFailed');
    });
});
