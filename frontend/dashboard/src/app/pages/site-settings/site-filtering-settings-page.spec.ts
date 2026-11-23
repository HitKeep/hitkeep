import { signal } from '@angular/core';
import { SiteService } from '@features/sites/services/site.service';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { By } from '@angular/platform-browser';
import { ExclusionEditor } from '@features/exclusions/exclusion-editor';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { of, Subject, throwError } from 'rxjs';
import { vi } from 'vitest';

import { SiteFilteringSettingsPage } from './site-filtering-settings-page';
import { ExclusionsService } from '@services/exclusions.service';
import { IPExclusion, Site } from '@models/analytics.types';

describe('SiteFilteringSettingsPage', () => {
    let editor: ExclusionEditor;
    let fixture: ComponentFixture<SiteFilteringSettingsPage>;

    const exclusionsService = {
        getCurrentIP: vi.fn(() => of({ ip: '203.0.113.10', cidr: '203.0.113.10/32' })),
        listSiteExclusions: vi.fn((siteID: string, effective: boolean) => {
            void siteID;
            void effective;
            return of<IPExclusion[]>([]);
        }),
        createSiteExclusion: vi.fn(() =>
            of<IPExclusion>({
                id: 'rule-1',
                type: 'cidr' as const,
                cidr: '203.0.113.10/32',
                description: 'Office',
                created_at: '2026-05-01T00:00:00Z'
            })
        ),
        deleteSiteExclusion: vi.fn()
    };

    const site: Site = {
        id: 'site-1',
        user_id: 'user-1',
        domain: 'example.com',
        created_at: '2026-05-01T00:00:00Z'
    };

    const activeSite = signal<Site | null>(site);

    beforeEach(async () => {
        vi.clearAllMocks();

        await TestBed.configureTestingModule({
            imports: [
                SiteFilteringSettingsPage,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: {
                                columns: { actions: 'Actions' },
                                actions: { cancel: 'Cancel' },
                                searchPlaceholder: 'Search',
                                copyControl: {
                                    copy: 'Copy',
                                    copied: 'Copied',
                                    failed: 'Copy failed',
                                    ariaLabel: 'Copy to clipboard'
                                }
                            },
                            settings: { apiClients: { actions: { refresh: 'Refresh' } } },
                            share: { dialog: { deleteAction: 'Delete' } },
                            sites: {
                                settings: { tabs: { filtering: 'Filtering' } },
                                exclusions: {
                                    suggestionsTitle: 'Suggestions',
                                    suggestionsHint: 'Use your current IP.',
                                    currentIpLoading: 'Loading current IP',
                                    currentIpUnavailable: 'Current IP unavailable',
                                    addAction: 'Add exclusion',
                                    description: 'Exclude traffic.',
                                    typeLabel: 'Rule type',
                                    cidrLabel: 'CIDR',
                                    cidrPlaceholder: '203.0.113.10/32',
                                    countryLabel: 'Country',
                                    countryPlaceholder: 'Select a country',
                                    userAgentLabel: 'User-agent contains',
                                    userAgentPlaceholder: 'monitoring-bot',
                                    pathLabel: 'Path',
                                    pathPlaceholder: '/admin',
                                    descriptionLabel: 'Description',
                                    descriptionPlaceholder: 'Office',
                                    loading: 'Loading',
                                    empty: 'No exclusions',
                                    confirmDelete: 'Delete {{value}}?',
                                    inheritedHint: 'Inherited filters apply.',
                                    inheritedReadOnly: 'Inherited',
                                    ruleTypes: { cidr: 'IP/CIDR', country: 'Country', userAgent: 'User agent', path: 'Path' },
                                    scopes: { instance: 'Instance', team: 'Team', site: 'Site' },
                                    columns: {
                                        type: 'Type',
                                        value: 'Value',
                                        description: 'Description',
                                        created: 'Created'
                                    },
                                    errors: {
                                        invalidCidr: 'Invalid CIDR',
                                        invalidCountry: 'Select a country',
                                        invalidUserAgent: 'Enter a user agent',
                                        invalidPath: 'Enter a path',
                                        descriptionTooLong: 'Too long',
                                        loadFailed: 'Load failed',
                                        createFailed: 'Create failed',
                                        deleteFailed: 'Delete failed'
                                    },
                                    status: {
                                        createSuccess: 'Created {{value}}',
                                        deleteSuccess: 'Deleted {{value}}'
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
                provideTranslocoLocale({
                    langToLocaleMapping: {
                        en: 'en-US'
                    }
                }),
                { provide: SiteService, useValue: { activeSite } },
                { provide: ExclusionsService, useValue: exclusionsService }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(SiteFilteringSettingsPage);
        activeSite.set(site);
        fixture.detectChanges();
        editor = fixture.debugElement.query(By.directive(ExclusionEditor)).componentInstance;
    });

    afterEach(() => {
        document.querySelectorAll('.p-dialog-mask, .p-confirm-dialog, .table-row-actions-menu').forEach((element) => element.remove());
    });

    it('renders the current IP with the shared copy control', () => {
        const text = fixture.nativeElement.textContent as string;
        const copyButton = fixture.nativeElement.querySelector('app-copy-control button') as HTMLButtonElement | null;

        expect(text).toContain('203.0.113.10/32');
        expect(copyButton).not.toBeNull();
        expect(copyButton?.textContent).toContain('Copy');
        expect(copyButton?.disabled).toBe(false);
    });

    it('requests effective rules and keeps inherited rows read-only', () => {
        const inherited: IPExclusion = {
            id: 'instance-rule',
            scope: 'instance',
            type: 'path',
            path: '/internal',
            inherited: true,
            created_at: '2026-05-01T00:00:00Z'
        };
        editor['exclusions'].set([inherited]);
        fixture.detectChanges();

        expect(exclusionsService.listSiteExclusions).toHaveBeenCalledWith('site-1', true);
        expect(fixture.nativeElement.textContent).toContain('/internal');
        expect(fixture.nativeElement.textContent).toContain('Instance');
        expect(fixture.nativeElement.textContent).toContain('Inherited');
        expect(fixture.nativeElement.querySelector('app-table-row-actions')).toBeNull();
        expect(editor['ruleActions'](inherited)).toEqual([]);
    });

    it('cancels the old list on site switch, and shows retry instead of an empty state after failure', () => {
        const oldRead = new Subject<IPExclusion[]>();
        exclusionsService.listSiteExclusions.mockReturnValueOnce(oldRead);
        editor['reload']();
        expect(oldRead.observed).toBe(true);
        exclusionsService.listSiteExclusions.mockReturnValueOnce(throwError(() => new Error('offline')));
        activeSite.set({ ...site, id: 'site-2' });
        fixture.detectChanges();
        expect(oldRead.observed).toBe(false);
        expect(exclusionsService.listSiteExclusions).toHaveBeenLastCalledWith('site-2', true);
        expect(fixture.nativeElement.textContent).toContain('Load failed');
        expect(fixture.nativeElement.textContent).not.toContain('No exclusions');
        clickButton('Refresh');
        fixture.detectChanges();
        expect(fixture.nativeElement.textContent).toContain('No exclusions');
        expect(fixture.nativeElement.textContent).not.toContain('Load failed');
    });

    it('discards a pending create and its dialog when switching sites', () => {
        const oldWrite = new Subject<IPExclusion>();
        exclusionsService.createSiteExclusion.mockReturnValueOnce(oldWrite);
        editor['openAddDialog']('192.0.2.1/32');
        editor['addRule']();
        activeSite.set({ ...site, id: 'site-2' });
        fixture.detectChanges();
        oldWrite.next({ id: 'old', type: 'cidr', cidr: '192.0.2.1/32', created_at: '2026-01-01T00:00:00Z' });
        expect(editor['isAddDialogVisible']()).toBe(false);
        expect(editor['isSaving']()).toBe(false);
        expect(editor['exclusions']()).toEqual([]);
        expect(editor['actionStatus']()).toBeNull();
    });

    it('closes an open deletion confirmation and ignores its old accept callback after switching sites', () => {
        const confirmation = fixture.debugElement.query(By.directive(ExclusionEditor)).injector.get(ConfirmationService);
        const confirm = vi.spyOn(confirmation, 'confirm');
        const close = vi.spyOn(confirmation, 'close');
        editor['confirmDeleteRule']({ id: 'old', type: 'user_agent', user_agent: 'Monitor <office>', created_at: '2026-01-01T00:00:00Z' });
        fixture.detectChanges();
        expect(document.body.textContent).toContain('Monitor <office>');
        const dialog = confirm.mock.calls[0][0];
        expect(dialog.header).toBe(site.domain);
        activeSite.set({ ...site, id: 'site-2', domain: 'second.example.test' });
        fixture.detectChanges();
        expect(close).toHaveBeenCalled();
        dialog.accept?.();
        expect(exclusionsService.deleteSiteExclusion).not.toHaveBeenCalled();
    });

    it('shows the exclusions table surface first and opens add form in a dialog', async () => {
        expect(fixture.nativeElement.querySelector('.site-settings-dialog-form')).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('Add exclusion');

        clickButton('Add exclusion');
        fixture.detectChanges();
        await fixture.whenStable();

        expect(document.body.querySelector('.site-settings-dialog-form')).not.toBeNull();
        expect(document.body.textContent).toContain('CIDR');
    });

    it('creates a site exclusion from the dialog and closes it', async () => {
        clickButton('Add exclusion');
        fixture.detectChanges();
        await fixture.whenStable();

        (document.body.querySelector('#site-exclusion-cidr') as HTMLInputElement).value = '203.0.113.10/32';
        (document.body.querySelector('#site-exclusion-cidr') as HTMLInputElement).dispatchEvent(new Event('input'));
        (document.body.querySelector('#site-exclusion-description') as HTMLInputElement).value = 'Office';
        (document.body.querySelector('#site-exclusion-description') as HTMLInputElement).dispatchEvent(new Event('input'));

        editor['addRule']();
        fixture.detectChanges();

        const createCalls = (exclusionsService.createSiteExclusion as unknown as { mock: { calls: unknown[][] } }).mock.calls;
        expect(createCalls[0]).toEqual([
            'site-1',
            {
                type: 'cidr',
                cidr: '203.0.113.10/32',
                country_code: undefined,
                user_agent: undefined,
                path: undefined,
                description: 'Office'
            }
        ]);
        expect(editor['isAddDialogVisible']()).toBe(false);
        expect(fixture.nativeElement.textContent).toContain('203.0.113.10/32');
        expect(fixture.nativeElement.textContent).toContain('Created 203.0.113.10/32');
    });

    it('shows row loading and success feedback while deleting a site exclusion', () => {
        const pending = new Subject<void>();
        exclusionsService.deleteSiteExclusion.mockReturnValueOnce(pending.asObservable());
        editor['exclusions'].set([
            {
                id: 'rule-1',
                type: 'cidr',
                cidr: '203.0.113.10/32',
                description: 'Office',
                created_at: '2026-05-01T00:00:00Z'
            }
        ]);
        fixture.detectChanges();

        editor['deleteRule']({
            id: 'rule-1',
            type: 'cidr',
            cidr: '203.0.113.10/32',
            description: 'Office',
            created_at: '2026-05-01T00:00:00Z'
        });
        fixture.detectChanges();

        expect(editor['deletingRuleID']()).toBe('rule-1');

        pending.next();
        pending.complete();
        fixture.detectChanges();

        expect(editor['deletingRuleID']()).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('Deleted 203.0.113.10/32');
    });

    it('ignores duplicate create submits while the request is in flight', async () => {
        const pending = new Subject<{ id: string; type: 'cidr'; cidr: string; description: string; created_at: string }>();
        exclusionsService.createSiteExclusion.mockReturnValueOnce(pending.asObservable());
        clickButton('Add exclusion');
        fixture.detectChanges();
        await fixture.whenStable();

        (document.body.querySelector('#site-exclusion-cidr') as HTMLInputElement).value = '203.0.113.10/32';
        (document.body.querySelector('#site-exclusion-cidr') as HTMLInputElement).dispatchEvent(new Event('input'));

        editor['addRule']();
        editor['addRule']();

        expect(exclusionsService.createSiteExclusion).toHaveBeenCalledTimes(1);
        pending.complete();
    });

    it('creates a site country exclusion from the dialog', async () => {
        exclusionsService.createSiteExclusion.mockReturnValueOnce(
            of<IPExclusion>({
                id: 'rule-country',
                type: 'country' as const,
                country_code: 'DE',
                description: 'Germany',
                created_at: '2026-05-01T00:00:00Z'
            })
        );
        clickButton('Add exclusion');
        fixture.detectChanges();
        await fixture.whenStable();

        editor['form'].controls.type.setValue('country');
        editor['form'].controls.countryCode.setValue('DE');
        editor['form'].controls.description.setValue('Germany');

        editor['addRule']();
        fixture.detectChanges();

        const createCalls = (exclusionsService.createSiteExclusion as unknown as { mock: { calls: unknown[][] } }).mock.calls;
        expect(createCalls.at(-1)).toEqual([
            'site-1',
            {
                type: 'country',
                cidr: undefined,
                country_code: 'DE',
                user_agent: undefined,
                path: undefined,
                description: 'Germany'
            }
        ]);
        expect(fixture.nativeElement.textContent).toContain('Germany (DE)');
    });

    it('keeps create errors inside the add dialog', async () => {
        exclusionsService.createSiteExclusion.mockReturnValueOnce(throwError(() => new Error('nope')));
        clickButton('Add exclusion');
        fixture.detectChanges();
        await fixture.whenStable();

        (document.body.querySelector('#site-exclusion-cidr') as HTMLInputElement).value = '203.0.113.10/32';
        (document.body.querySelector('#site-exclusion-cidr') as HTMLInputElement).dispatchEvent(new Event('input'));

        editor['addRule']();
        fixture.detectChanges();
        await fixture.whenStable();

        expect(document.body.querySelector('.p-dialog')?.textContent).toContain('Create failed');
        expect(fixture.nativeElement.textContent).not.toContain('Create failed');
    });

    function clickButton(label: string): void {
        const button = Array.from(fixture.nativeElement.querySelectorAll('button') as NodeListOf<HTMLButtonElement>).find((entry) => entry.textContent?.includes(label));
        button?.click();
    }
});
