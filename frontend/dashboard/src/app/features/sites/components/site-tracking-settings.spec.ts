import { Clipboard } from '@angular/cdk/clipboard';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoService, TranslocoTestingModule } from '@jsverse/transloco';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { SiteService } from '@features/sites/services/site.service';
import de from '../../../../../public/i18n/de.json';
import en from '../../../../../public/i18n/en.json';
import { InstallMethod, SiteTrackingSettings } from './site-tracking-settings';

interface Toggle {
    control: () => { setValue: (value: boolean) => void };
}

type Internals = SiteTrackingSettings & {
    snippetCode: () => string;
    trackerHostURL: () => string;
    npmInitSnippet: () => string;
    serverIngestSnippet: () => string;
    changedOptionCount: () => number;
    selectInstallMethod: (method: InstallMethod) => void;
    trackingDomainControl: { setValue: (value: string | null) => void };
    trackingForm: {
        collectDnt: () => Toggle;
        disableBeacon: () => Toggle;
        enableWebVitals: () => Toggle;
        enablePaidSignals: () => Toggle;
        trackOutbound: () => Toggle;
        trackDownloads: () => Toggle;
        trackForms: () => Toggle;
        enableDeclarativeEvents: () => Toggle;
    };
};

const customDomainOptions = {
    site_id: 'site-1',
    team_id: 'team-1',
    default_url: 'https://hitkeep.test/hk.js',
    domains: [
        {
            id: 'domain-1',
            team_id: 'team-1',
            hostname: 'analytics.example.com',
            verification_status: 'verified',
            target_status: 'verified',
            tls_mode: 'external',
            tls_status: 'verified',
            enabled: true,
            active: true,
            dns_txt_name: '_hitkeep-tracking.analytics.example.com',
            dns_txt_value: 'hitkeep-domain-verification=token',
            dns_target: 'hitkeep.test',
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z'
        }
    ]
};

const site = {
    id: 'site-1',
    user_id: 'user-1',
    domain: 'example.com',
    created_at: '2026-01-01T00:00:00Z'
};

describe('SiteTrackingSettings', () => {
    let component: SiteTrackingSettings;
    let internals: Internals;
    let fixture: ComponentFixture<SiteTrackingSettings>;
    let base: HTMLBaseElement;
    let previousBases: HTMLBaseElement[];
    let siteServiceMock: {
        getTrackingStatus: ReturnType<typeof vi.fn>;
        getTrackingDomainOptions: ReturnType<typeof vi.fn>;
    };

    const testId = (id: string) => fixture.nativeElement.querySelector(`[data-testid="${id}"]`) as HTMLElement | null;

    /** Mount the site with one active custom tracking domain and select it. */
    const selectCustomDomain = () => {
        siteServiceMock.getTrackingDomainOptions.mockReturnValue(of(customDomainOptions));
        fixture.componentRef.setInput('site', site);
        fixture.detectChanges();
        internals.trackingDomainControl.setValue('domain-1');
        fixture.detectChanges();
    };

    beforeEach(async () => {
        previousBases = Array.from(window.document.head.querySelectorAll('base'));
        previousBases.forEach((entry) => entry.remove());
        base = window.document.createElement('base');
        base.href = '/hitkeep/';
        window.document.head.append(base);
        siteServiceMock = {
            getTrackingStatus: vi.fn(() =>
                of({
                    site_id: 'site-1',
                    tenant_id: 'team-1',
                    status: 'waiting',
                    configured_domain: 'example.com'
                })
            ),
            getTrackingDomainOptions: vi.fn(() =>
                of({
                    site_id: 'site-1',
                    team_id: 'team-1',
                    default_url: 'https://hitkeep.test/hk.js',
                    domains: []
                })
            )
        };

        await TestBed.configureTestingModule({
            imports: [
                SiteTrackingSettings,
                TranslocoTestingModule.forRoot({
                    langs: { en, de },
                    translocoConfig: {
                        availableLangs: ['en', 'de'],
                        defaultLang: 'en',
                        reRenderOnLangChange: true
                    },
                    preloadLangs: true
                })
            ],
            providers: [{ provide: SiteService, useValue: siteServiceMock }]
        }).compileComponents();

        fixture = TestBed.createComponent(SiteTrackingSettings);
        fixture.componentRef.setInput('site', null);
        component = fixture.componentInstance;
        internals = component as Internals;
        fixture.detectChanges();
    });
    afterEach(() => {
        vi.restoreAllMocks();
        base.remove();
        previousBases.forEach((entry) => window.document.head.append(entry));
    });
    it('should create', () => {
        expect(component).toBeTruthy();
    });
    it('should update snippet when toggles change', () => {
        const getSnippet = () => internals.snippetCode();

        expect(getSnippet()).toContain('/hitkeep/hk.js');

        expect(getSnippet()).not.toContain('data-collect-dnt');
        expect(getSnippet()).not.toContain('data-disable-beacon');
        expect(getSnippet()).not.toContain('data-enable-web-vitals');
        expect(getSnippet()).not.toContain('data-disable-outbound-tracking');
        expect(getSnippet()).not.toContain('data-disable-download-tracking');
        expect(getSnippet()).not.toContain('data-disable-form-tracking');

        internals.trackingForm.collectDnt().control().setValue(true);
        internals.trackingForm.enableWebVitals().control().setValue(true);
        fixture.detectChanges();
        expect(getSnippet()).toContain('data-collect-dnt="true"');
        expect(getSnippet()).toContain('data-enable-web-vitals="true"');

        internals.trackingForm.disableBeacon().control().setValue(true);
        internals.trackingForm.trackOutbound().control().setValue(false);
        internals.trackingForm.trackDownloads().control().setValue(false);
        internals.trackingForm.trackForms().control().setValue(false);
        fixture.detectChanges();
        expect(getSnippet()).toContain('hk.js');
        expect(getSnippet()).toContain('data-disable-beacon="true"');
        expect(getSnippet()).toContain('data-disable-outbound-tracking="true"');
        expect(getSnippet()).toContain('data-disable-download-tracking="true"');
        expect(getSnippet()).toContain('data-disable-form-tracking="true"');
    });

    it('should use a selected custom tracking domain in the snippet', () => {
        selectCustomDomain();
        expect(internals.snippetCode()).toContain('src="https://analytics.example.com/hk.js"');
    });

    it('suppresses the referrer on the script request and keeps paid signals opt-in', async () => {
        const defaultSnippet = internals.snippetCode();
        expect(defaultSnippet).not.toContain('data-enable-paid-signals');
        expect(defaultSnippet).toContain('referrerpolicy="no-referrer"');
        expect(internals.changedOptionCount()).toBe(0);

        internals.trackingForm.enablePaidSignals().control().setValue(true);
        await fixture.whenStable();

        const renderedSnippet = testId('tracking-snippet')?.querySelector('pre')?.textContent;
        expect(renderedSnippet).toContain('data-enable-paid-signals="true"');
        expect(renderedSnippet).toContain('referrerpolicy="no-referrer"');
        expect(internals.changedOptionCount()).toBe(1);

        internals.trackingForm.enablePaidSignals().control().setValue(false);
        await fixture.whenStable();
        expect(internals.snippetCode()).toBe(defaultSnippet);
    });

    it('copies the current script and localized verification instructions on demand', async () => {
        const copy = vi.spyOn(TestBed.inject(Clipboard), 'copy').mockReturnValue(true);
        const getButton = () => testId('tracking-copy-assistant-prompt')?.querySelector('button');
        const getShownSnippet = () => testId('tracking-snippet')?.querySelector('pre')?.textContent;

        expect(getButton()).toBeUndefined();
        siteServiceMock.getTrackingDomainOptions.mockReturnValue(of(customDomainOptions));
        fixture.componentRef.setInput('site', site);
        await fixture.whenStable();

        expect(getButton()?.getAttribute('aria-label')).toBe(en.sites.tracking.install.assistant.copyAction);
        expect(copy).not.toHaveBeenCalled();
        const defaultSnippet = getShownSnippet();
        expect(defaultSnippet).toContain('/hitkeep/hk.js');
        expect(defaultSnippet).not.toContain('data-enable-declarative-events');
        expect(defaultSnippet).toContain('referrerpolicy="no-referrer"');
        getButton()?.click();
        expect(copy).toHaveBeenCalledExactlyOnceWith(expect.stringContaining(`\n\n${defaultSnippet}\n\n`));
        expect(copy.mock.calls[0][0]).toContain('on example.com.');
        expect(copy.mock.calls[0][0]).toContain(en.sites.tracking.verifier.title);

        TestBed.inject(TranslocoService).setActiveLang('de');
        await fixture.whenStable();
        expect(getButton()?.getAttribute('aria-label')).toBe(de.sites.tracking.install.assistant.copyAction);

        internals.trackingDomainControl.setValue('domain-1');
        internals.trackingForm.trackForms().control().setValue(false);
        internals.trackingForm.enableDeclarativeEvents().control().setValue(true);
        internals.trackingForm.enablePaidSignals().control().setValue(true);
        await fixture.whenStable();

        const customSnippet = getShownSnippet();
        expect(customSnippet).toContain('src="https://analytics.example.com/hk.js" data-disable-form-tracking="true"');
        expect(customSnippet).toContain('data-enable-declarative-events="true"');
        expect(customSnippet).toContain('data-enable-paid-signals="true" referrerpolicy="no-referrer"');
        expect(internals.changedOptionCount()).toBe(4);
        getButton()?.click();
        expect(copy).toHaveBeenCalledTimes(2);
        expect(copy).toHaveBeenLastCalledWith(expect.stringContaining(`\n\n${customSnippet}\n\n`));
        expect(copy.mock.calls[1][0]).toContain(de.sites.tracking.verifier.title);
        expect(copy.mock.calls[1][0]).not.toMatch(/site-1|team-1|domain-1|user-1|hitkeep-domain-verification|HITKEEP_API_TOKEN/);
    });

    it('derives the instance host by stripping the script filename, keeping the base path prefix', () => {
        expect(internals.trackerHostURL()).toMatch(/\/hitkeep$/);
        expect(internals.trackerHostURL()).not.toContain('hk.js');

        selectCustomDomain();

        expect(internals.trackerHostURL()).toBe('https://analytics.example.com');
        expect(internals.npmInitSnippet()).toContain("init({ host: 'https://analytics.example.com' })");
        expect(internals.serverIngestSnippet()).toContain('https://analytics.example.com/api/ingest/server/pageview');
    });

    it('puts the site domain in the server-side ingest sample', () => {
        fixture.componentRef.setInput('site', site);
        fixture.detectChanges();

        expect(internals.serverIngestSnippet()).toContain('"url": "https://example.com/pricing"');
        expect(internals.serverIngestSnippet()).toContain('Authorization: Bearer $HITKEEP_API_TOKEN');
    });

    it('renders the selected install method and only shows advanced options for the script tag', () => {
        expect(testId('tracking-snippet')).toBeTruthy();
        expect(testId('tracking-advanced-options')).toBeTruthy();

        internals.selectInstallMethod('npm');
        fixture.detectChanges();
        expect(testId('tracking-snippet')).toBeNull();
        expect(testId('tracking-copy-assistant-prompt')).toBeNull();
        expect(testId('tracking-advanced-options')).toBeNull();
        expect(testId('tracking-npm-install')).toBeTruthy();
        expect(testId('tracking-npm-registry-link')?.getAttribute('href')).toBe('https://www.npmjs.com/package/@hitkeep/tracker');

        internals.selectInstallMethod('wordpress');
        fixture.detectChanges();
        expect(testId('tracking-npm-install')).toBeNull();
        expect(testId('tracking-wordpress-url')?.textContent).toContain('/hitkeep');
        expect(testId('tracking-wordpress-directory-link')?.getAttribute('href')).toBe('https://wordpress.org/plugins/hitkeep/');

        internals.selectInstallMethod('server');
        fixture.detectChanges();
        expect(testId('tracking-wordpress-url')).toBeNull();
        expect(testId('tracking-server-snippet')).toBeTruthy();
        expect(testId('tracking-server-docs-link')?.getAttribute('href')).toBe('https://hitkeep.com/guides/tracking/server-side-tracking/');

        internals.selectInstallMethod('script');
        fixture.detectChanges();
        expect(testId('tracking-snippet')).toBeTruthy();
    });

    it('counts only non-default snippet options', () => {
        expect(internals.changedOptionCount()).toBe(0);
        expect(testId('tracking-options-badge')).toBeNull();

        internals.trackingForm.collectDnt().control().setValue(true);
        internals.trackingForm.trackOutbound().control().setValue(false);
        fixture.detectChanges();
        expect(internals.changedOptionCount()).toBe(2);
        expect(testId('tracking-options-badge')).toBeTruthy();

        internals.trackingForm.collectDnt().control().setValue(false);
        internals.trackingForm.trackOutbound().control().setValue(true);
        fixture.detectChanges();
        expect(internals.changedOptionCount()).toBe(0);
    });

    it('counts a selected custom tracking domain as a changed option', () => {
        expect(internals.changedOptionCount()).toBe(0);

        selectCustomDomain();

        expect(internals.changedOptionCount()).toBe(1);
    });

    it('reveals the verifier field grid on demand', () => {
        fixture.componentRef.setInput('site', site);
        fixture.detectChanges();

        expect(testId('tracking-verifier-details')).toBeNull();

        const toggle = testId('tracking-verifier-toggle');
        (toggle?.querySelector('button') ?? toggle)?.click();
        fixture.detectChanges();

        expect(testId('tracking-verifier-details')).toBeTruthy();
    });
});
