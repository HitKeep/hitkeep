import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { TeamOverviewPage } from './team-overview';
import { TeamService } from '@services/team.service';
import { AnalyticsService } from '@services/analytics.service';
import { CloudService } from '@services/cloud.service';
import { OAuthService } from '@services/oauth.service';
import { Team } from '@models/analytics.types';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';

describe('TeamOverviewPage', () => {
    const oauth = { resuming: signal(false), continuation: vi.fn(() => of({ pending: false })), continueAfterAuthentication: vi.fn() };
    let fixture: ComponentFixture<TeamOverviewPage>;
    let catalogCurrency = 'EUR';
    let routeQuery = convertToParamMap({});
    let multipleTeams = false;
    let billingState: { team_id: string; plan_code: string; subscription_status: string; pending_plan_code: string; billing: string; promotion?: import('@services/cloud.service').CloudBillingState['promotion'] } = {
        team_id: '00000000-0000-0000-0000-000000000001',
        plan_code: 'free',
        subscription_status: 'free',
        pending_plan_code: '',
        billing: ''
    };
    type TeamOverviewTestAccess = TeamOverviewPage & {
        openBillingPortal(): void;
        startUpgradeCheckout(): void;
        startRecoveryCheckout(): void;
        continueOnFree(): void;
        confirmRecoveryTeam(): void;
        recoveryKind(): string | null;
        recoveryIntent(): { plan: string; billing: string };
        redirectTo(url: string): void;
    };
    let systemStatusResponse: {
        needs_setup: boolean;
        version: string;
        cloud?: {
            hosted: boolean;
            signup_enabled: boolean;
            jurisdiction?: string;
            region?: string;
            upgrade_url?: string;
            support_url?: string;
        };
    };

    const createActiveTeam = (): Team => ({
        id: '00000000-0000-0000-0000-000000000001',
        name: 'Acme Analytics',
        logo_url: '',
        role: 'owner',
        created_at: '2026-03-01T00:00:00Z',
        usage: {
            current_sites: 3,
            current_members: 5,
            current_pending_invites: 2
        },
        entitlements: {
            max_sites_per_team: 10,
            max_team_members: 8,
            max_retention_days: 365,
            allow_sso: true,
            allow_custom_branding: true,
            allow_external_report_recipients: false
        },
        plan: {
            code: 'free',
            name: 'Free',
            upgrade_url: 'https://hitkeep.com/cloud/upgrade',
            support_url: 'https://hitkeep.com/cloud/support'
        }
    });
    const activeTeam = signal<Team | null>(createActiveTeam());

    beforeEach(async () => {
        oauth.continuation.mockReset().mockReturnValue(of({ pending: false }));
        oauth.continueAfterAuthentication.mockReset();
        catalogCurrency = 'EUR';
        routeQuery = convertToParamMap({});
        multipleTeams = false;
        billingState = { team_id: '00000000-0000-0000-0000-000000000001', plan_code: 'free', subscription_status: 'free', pending_plan_code: '', billing: '' };
        activeTeam.set(createActiveTeam());
        systemStatusResponse = {
            needs_setup: false,
            version: 'v2.0.0',
            cloud: {
                hosted: true,
                signup_enabled: false,
                jurisdiction: 'EU',
                region: 'eu-central-1',
                upgrade_url: 'https://hitkeep.com/cloud/upgrade',
                support_url: 'https://hitkeep.com/cloud/support'
            }
        };

        await TestBed.configureTestingModule({
            imports: [
                TeamOverviewPage,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            admin: {
                                team: {
                                    overview: {
                                        title: 'Team',
                                        teamIdLabel: 'Team ID',
                                        memberCount: 'Members',
                                        usage: {
                                            title: 'Usage and limits',
                                            description: 'Track plan usage before it becomes a limit.',
                                            currentUsage: '{{count}} used',
                                            unlimited: 'Unlimited',
                                            unlimitedFootnote: 'Unlimited on the current plan',
                                            limitValue: 'Limit {{count}}',
                                            progressLabel: '{{count}}% of limit',
                                            pendingInviteOne: '1 pending invite',
                                            pendingInviteMany: '{{count}} pending invites',
                                            metrics: {
                                                sites: 'Sites',
                                                members: 'Members'
                                            }
                                        },
                                        cloud: {
                                            title: 'Cloud plan',
                                            managed: 'Managed cloud',
                                            description: 'Run this team in the hosted EU or US cell with hard limits, backups, and upgrade paths.',
                                            operatorPlan: 'Operator',
                                            operatorDescription: 'This operator-owned team is internal to HitKeep Cloud and is not limited by customer plan entitlements.',
                                            manageBillingAction: 'Manage billing',
                                            jurisdiction: 'Jurisdiction',
                                            region: 'Region',
                                            retention: 'Retention',
                                            retentionDays: '{{count}} days',
                                            unlimitedRetention: 'Unlimited retention',
                                            upgradeAction: 'Upgrade plan',
                                            supportAction: 'Contact support'
                                        },
                                        plans: {
                                            title: 'Upgrade your plan',
                                            description: 'Unlock longer retention, more sites, and team capacity.',
                                            currentPlan: 'Current plan',
                                            recommended: 'Recommended',
                                            upgradeAction: 'Upgrade to {{plan}}',
                                            features: {
                                                sites: 'Up to {{count}} sites',
                                                members: 'Up to {{count}} team members',
                                                retention: '{{count}}-year data retention',
                                                retentionDays: '{{count}}-day data retention',
                                                externalReportRecipients: 'External scheduled-report recipients',
                                                customDomains: 'Custom tracking domains',
                                                sso: 'Single sign-on (SSO)'
                                            }
                                        }
                                    }
                                }
                            },
                            common: {
                                columns: { role: 'Role' },
                                copyControl: {
                                    copy: 'Copy',
                                    copied: 'Copied',
                                    failed: 'Copy failed',
                                    ariaLabel: 'Copy to clipboard'
                                }
                            },
                            cloud: {
                                billing: { annualTotal: 'Billed {{price}} per year', activated: 'Your {{plan}} plan is active!', processing: 'Payment processing' },
                                promotion: { saving: '{{percent}} off one invoice', withCode: 'with {{code}}', annualRenewal: 'Renews at {{price}} per year.', deadline: 'Start checkout before {{date}}.' }
                            },
                            teams: { roles: { owner: 'Owner' } },
                            signup: {
                                plans: {
                                    free: {
                                        name: 'Free',
                                        description: '60-day retention to validate your sites.'
                                    },
                                    pro: {
                                        name: 'Pro',
                                        description: 'Longer retention and more room for a small team.'
                                    },
                                    business: {
                                        name: 'Business',
                                        description: 'More scale for agencies.'
                                    }
                                }
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    }
                })
            ],
            providers: [
                { provide: OAuthService, useValue: oauth },
                provideRouter([]),
                provideTranslocoLocale({ langToLocaleMapping: { en: 'en-US' } }),
                {
                    provide: ActivatedRoute,
                    useValue: {
                        snapshot: {
                            get queryParamMap() {
                                return routeQuery;
                            }
                        }
                    }
                },
                {
                    provide: TeamService,
                    useValue: {
                        activeTeam,
                        loadTeams: vi.fn(() => of(undefined)),
                        activeTeamId: () => activeTeam()?.id ?? '',
                        hasMultipleTeams: () => multipleTeams
                    }
                },
                {
                    provide: AnalyticsService,
                    useValue: {
                        getSystemStatus: () => of(systemStatusResponse)
                    }
                },
                {
                    provide: CloudService,
                    useValue: {
                        createBillingPortalSession: vi.fn().mockReturnValue(of({ url: 'https://billing.stripe.test/session' })),
                        createBillingCheckoutSession: vi.fn().mockReturnValue(of({ url: 'https://checkout.stripe.test/session' })),
                        redeemPromotion: vi.fn().mockReturnValue(of(undefined)),
                        watchBillingState: vi.fn(() => of(billingState)),
                        refreshBillingState: vi.fn(),
                        getBillingState: vi.fn().mockImplementation(() => of(billingState)),
                        getPlans: vi.fn().mockImplementation(() =>
                            of([
                                {
                                    code: 'free',
                                    name: 'Free',
                                    entitlements: {
                                        max_sites_per_team: 3,
                                        max_team_members: 3,
                                        max_retention_days: 60,
                                        allow_sso: false,
                                        allow_custom_branding: false,
                                        allow_external_report_recipients: false
                                    }
                                },
                                {
                                    code: 'pro',
                                    name: 'Pro',
                                    prices: { annual: { amount_minor: 15000, currency: catalogCurrency }, monthly: { amount_minor: 1500, currency: catalogCurrency } },
                                    entitlements: {
                                        max_sites_per_team: 10,
                                        max_team_members: 5,
                                        max_retention_days: 365,
                                        allow_sso: false,
                                        allow_custom_branding: false,
                                        allow_external_report_recipients: true
                                    }
                                },
                                {
                                    code: 'business',
                                    name: 'Business',
                                    prices: { annual: { amount_minor: 39000, currency: catalogCurrency }, monthly: { amount_minor: 3900, currency: catalogCurrency } },
                                    entitlements: {
                                        max_sites_per_team: 50,
                                        max_team_members: 20,
                                        max_retention_days: 1095,
                                        allow_sso: true,
                                        allow_custom_branding: true,
                                        allow_external_report_recipients: true
                                    }
                                }
                            ])
                        )
                    }
                }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(TeamOverviewPage);
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();
    });

    it('renders usage cards for the active team', () => {
        const text = fixture.nativeElement.textContent;
        expect(text).toContain('Usage and limits');
        expect(text).toContain('Sites');
        expect(text).toContain('Members');
        expect(text).toContain('2 pending invites');
    });

    it.each([
        [7999, 'healthy'],
        [8000, 'warning'],
        [9499, 'warning'],
        [9500, 'critical']
    ])('marks monthly usage at %i hits as %s', (hits, severity) => {
        const team = createActiveTeam();
        activeTeam.set({ ...team, entitlements: { ...team.entitlements!, max_monthly_hits: 10000 }, usage: { ...team.usage!, current_monthly_hits: Number(hits) } });
        const card = fixture.componentInstance['usageCards']().find((card) => card.key === 'monthlyHits');
        expect(card?.className).toContain('--' + severity);
        expect(card?.limitLabel).toContain('10,000');
    });

    it('renders a singular pending invite label', () => {
        activeTeam.set({
            ...createActiveTeam(),
            usage: {
                current_sites: 3,
                current_members: 5,
                current_pending_invites: 1
            }
        });
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('1 pending invite');
    });

    it('renders the active team ID with a copy control', () => {
        const text = fixture.nativeElement.textContent;
        const copyButton = fixture.nativeElement.querySelector('app-copy-control button') as HTMLButtonElement | null;

        expect(text).toContain('Team ID');
        expect(text).toContain('00000000-0000-0000-0000-000000000001');
        expect(copyButton).not.toBeNull();
        expect(copyButton?.getAttribute('aria-label')).toBe('Copy to clipboard');
    });

    it('renders cloud plan with inline upgrade comparison for free users', () => {
        const text = fixture.nativeElement.textContent;
        expect(text).toContain('Cloud plan');
        expect(text).toContain('Managed cloud');
        expect(text).toContain('EU');
        expect(text).toContain('365 days');
        expect(text).toContain('Current plan');
        expect(text).toContain('Upgrade to Pro');
        expect(text).toContain('Upgrade to Business');
        expect(text).toContain('Single sign-on (SSO)');
        expect(text).toContain('External scheduled-report recipients');
        expect(text).toContain('€12.50');
        expect(text).toContain('Billed €150');
        expect(text).toContain('€390');
    });

    it('formats the catalog annual tiers in USD for the US jurisdiction', async () => {
        fixture.destroy();
        catalogCurrency = 'USD';
        systemStatusResponse.cloud = {
            ...systemStatusResponse.cloud!,
            jurisdiction: 'US',
            region: 'us-east-1'
        };

        fixture = TestBed.createComponent(TeamOverviewPage);
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        const text = fixture.nativeElement.textContent;
        expect(text).toContain('$150');
        expect(text).toContain('$390');
        expect(text).not.toContain('€150');
    });

    it('renders Business with SSO as the next upgrade for Pro users', async () => {
        activeTeam.set({
            ...createActiveTeam(),
            plan: { code: 'pro', name: 'Pro' },
            entitlements: { ...createActiveTeam().entitlements!, allow_sso: false }
        });
        await fixture.whenStable();

        const text = fixture.nativeElement.textContent;
        expect(text).toContain('Current plan');
        expect(text).toContain('Upgrade to Business');
        expect(text).toContain('Single sign-on (SSO)');
        expect(text).not.toContain('Upgrade to Pro');
    });

    it('renders operator-owned cloud teams as internal and unlimited', () => {
        activeTeam.set({
            id: '00000000-0000-0000-0000-000000000003',
            name: 'Operator Team',
            logo_url: '',
            role: 'owner',
            created_at: '2026-03-01T00:00:00Z',
            usage: {
                current_sites: 12,
                current_members: 8,
                current_pending_invites: 0
            },
            entitlements: {
                max_sites_per_team: 0,
                max_team_members: 0,
                max_retention_days: 0,
                allow_sso: true,
                allow_custom_branding: true,
                allow_external_report_recipients: true
            },
            plan: {
                code: 'operator',
                name: 'Operator'
            }
        });
        fixture.detectChanges();

        const text = fixture.nativeElement.textContent;
        expect(text).toContain('Operator');
        expect(text).toContain('internal to HitKeep Cloud');
        expect(text).toContain('Unlimited retention');
        expect(text).toContain('Unlimited on the current plan');
        expect(text).not.toContain('Manage billing');
        expect(text).not.toContain('Upgrade to Pro');
    });

    it('creates a billing portal session from the cloud card', () => {
        const component = fixture.componentInstance as TeamOverviewTestAccess;
        const cloudService = TestBed.inject(CloudService);
        const redirectSpy = vi.spyOn(component, 'redirectTo').mockImplementation(() => undefined);

        component.openBillingPortal();

        expect(cloudService.createBillingPortalSession).toHaveBeenCalledWith({
            locale: 'en'
        });
        expect(redirectSpy).toHaveBeenCalledWith('https://billing.stripe.test/session');
    });

    it('starts a checkout session when upgrading a free cloud team', () => {
        const component = fixture.componentInstance as TeamOverviewTestAccess;
        const cloudService = TestBed.inject(CloudService);
        const redirectSpy = vi.spyOn(component, 'redirectTo').mockImplementation(() => undefined);

        component.startUpgradeCheckout();

        expect(cloudService.createBillingCheckoutSession).toHaveBeenCalledWith({
            plan_code: 'pro',
            billing: 'annual',
            locale: 'en'
        });
        expect(redirectSpy).toHaveBeenCalledWith('https://checkout.stripe.test/session');
    });

    async function showRecovery(params: Record<string, string>, state: typeof billingState, hasMultipleTeams = false): Promise<TeamOverviewTestAccess> {
        fixture.destroy();
        routeQuery = convertToParamMap(params);
        billingState = state;
        multipleTeams = hasMultipleTeams;
        fixture = TestBed.createComponent(TeamOverviewPage);
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();
        return fixture.componentInstance as TeamOverviewTestAccess;
    }

    it('reviews and submits the team-bound first-invoice offer in its currency', async () => {
        const state = { ...billingState, promotion: { code: 'HKBF26', percent_off: 30, ends_at: new Date(Date.now() + 60000).toISOString(), prices: { business: { annual: { with_code_minor: 27300, regular_minor: 39000, currency: 'USD' } } } } };
        const component = await showRecovery({ purchase: 'review', plan: 'business', billing: 'annual' }, state);
        const offer = fixture.nativeElement.querySelector('app-promotion-price');
        expect(offer.querySelector('strong').textContent).toBe('$273.00');
        expect(offer.textContent).toContain('Renews at $390.00 per year.');
        vi.spyOn(component, 'redirectTo').mockImplementation(() => undefined);
        component.startRecoveryCheckout();
        expect(TestBed.inject(CloudService).createBillingCheckoutSession).toHaveBeenCalledWith({ plan_code: 'business', billing: 'annual', locale: 'en', team_id: state.team_id, expected_promotion: 'HKBF26' });
    });

    it('redeems a paid monthly invoice only after entering the code and submitting', async () => {
        const team = createActiveTeam();
        activeTeam.set({ ...team, plan: { ...team.plan!, code: 'pro', name: 'Pro' } });
        const state = {
            ...billingState,
            plan_code: 'pro',
            subscription_status: 'active',
            promotion: { code: 'HKBF26', percent_off: 30, ends_at: new Date(Date.now() + 60000).toISOString(), prices: { pro: { monthly: { with_code_minor: 1050, regular_minor: 1500, currency: 'USD' } } } }
        };
        await showRecovery({ team: team.id }, state);
        const service = TestBed.inject(CloudService);
        const input = fixture.nativeElement.querySelector('#promotion-code') as HTMLInputElement;
        expect(input.value).toBe('');
        expect(service.redeemPromotion).not.toHaveBeenCalled();
        input.value = 'HKBF26';
        input.form!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
        expect(service.redeemPromotion).toHaveBeenCalledWith(team.id, 'HKBF26');
        expect(service.createBillingCheckoutSession).not.toHaveBeenCalled();
    });

    it('keeps an explicit review selection when a different checkout is pending', async () => {
        const state = { ...billingState, subscription_status: 'pending_checkout', pending_plan_code: 'business', billing: 'monthly' };
        const review = await showRecovery({ purchase: 'review', plan: 'pro', billing: 'annual' }, state);
        expect(review.recoveryIntent()).toEqual({ plan: 'pro', billing: 'annual' });
        const canceled = await showRecovery({ checkout: 'canceled', team: state.team_id }, state);
        expect(canceled.recoveryIntent()).toEqual({ plan: 'business', billing: 'monthly' });
    });

    it('requires the team from an annual offer link before allowing review', async () => {
        const component = await showRecovery({ purchase: 'review', plan: 'pro', billing: 'annual', team: 'another-team' }, billingState);
        expect(component.recoveryKind()).toBe('switchTeam');
        component.startRecoveryCheckout();
        expect(TestBed.inject(CloudService).createBillingCheckoutSession).not.toHaveBeenCalled();
    });

    it('offers an explicit consent return only after authoritative paid state and a valid pending request', async () => {
        oauth.continuation.mockReturnValue(of({ pending: true }));
        await showRecovery({ checkout: 'success' }, { ...billingState, plan_code: 'pro', subscription_status: 'active' });
        expect(oauth.continueAfterAuthentication).not.toHaveBeenCalled();
        const button = fixture.nativeElement.querySelector('[data-testid="oauth-checkout-continue"] button') as HTMLButtonElement;
        expect(button).not.toBeNull();
        button.click();
        expect(oauth.continueAfterAuthentication).toHaveBeenCalledWith('/admin/team/overview');
    });

    it('confirms a delayed payment from synced Stripe state without OAuth messaging', async () => {
        await showRecovery({ checkout: 'success' }, { ...billingState, plan_code: 'free' });
        expect(fixture.nativeElement.textContent).toContain('cloud.billing.confirming');
        billingState = { ...billingState, plan_code: 'pro', subscription_status: 'active' };
        await vi.waitFor(() => expect(fixture.nativeElement.textContent).toContain('Your Pro plan is active!'), { timeout: 7000 });
        expect(fixture.nativeElement.textContent).not.toContain('oauth.');
        expect(TestBed.inject(TeamService).loadTeams).toHaveBeenCalledTimes(1);
    });

    it('does not treat a forged checkout success marker as payment or authorization', async () => {
        oauth.continuation.mockReturnValue(of({ pending: true }));
        await showRecovery({ checkout: 'success' }, { ...billingState, plan_code: 'free', subscription_status: 'pending_checkout' });
        expect(TestBed.inject(CloudService).getBillingState).toHaveBeenCalledWith({ sync: true });
        expect(fixture.nativeElement.textContent).toContain('Payment processing');
        expect(oauth.continuation).not.toHaveBeenCalled();
        expect(oauth.continueAfterAuthentication).not.toHaveBeenCalled();
        expect(fixture.nativeElement.querySelector('[data-testid="oauth-checkout-continue"]')).toBeNull();
    });

    it('keeps native billing visible when pending-authorization lookup fails', async () => {
        oauth.continuation.mockReturnValue(throwError(() => ({ status: 503, error: 'private details' })));
        await showRecovery({ checkout: 'success' }, { ...billingState, plan_code: 'pro', subscription_status: 'active' });
        expect(fixture.nativeElement.textContent).toContain('Your Pro plan is active!');
        expect(fixture.nativeElement.textContent).not.toContain('oauth.resumeFailed');
        expect(fixture.nativeElement.textContent).toContain('Acme Analytics');
        expect(fixture.nativeElement.textContent).not.toContain('private details');
        expect(oauth.continueAfterAuthentication).not.toHaveBeenCalled();
    });

    it('restores canceled Pro monthly from stored state and waits for an explicit retry', async () => {
        const component = await showRecovery({ checkout: 'canceled', plan: 'pro', billing: 'annual', team: billingState.team_id }, { ...billingState, subscription_status: 'pending_checkout', pending_plan_code: 'pro', billing: 'monthly' });
        const cloudService = TestBed.inject(CloudService);
        const redirectSpy = vi.spyOn(component, 'redirectTo').mockImplementation(() => undefined);

        expect(component.recoveryKind()).toBe('canceled');
        expect(component.recoveryIntent()).toEqual({ plan: 'pro', billing: 'monthly' });
        expect(cloudService.createBillingCheckoutSession).not.toHaveBeenCalled();

        component.startRecoveryCheckout();
        expect(cloudService.createBillingCheckoutSession).toHaveBeenCalledWith({ plan_code: 'pro', billing: 'monthly', locale: 'en' });
        expect(redirectSpy).toHaveBeenCalledWith('https://checkout.stripe.test/session');
    });

    it('does not treat a cancellation query as payment state', async () => {
        const component = await showRecovery({ checkout: 'canceled', plan: 'business', billing: 'annual', team: billingState.team_id }, billingState);
        expect(component.recoveryKind()).toBe('unavailable');
        component.startRecoveryCheckout();
        expect(TestBed.inject(CloudService).createBillingCheckoutSession).not.toHaveBeenCalled();
    });

    it('shows an active subscription without another purchase or Free continuation', async () => {
        const component = await showRecovery({ checkout: 'canceled', plan: 'pro', billing: 'monthly', team: billingState.team_id }, { ...billingState, plan_code: 'pro', subscription_status: 'active' });
        expect(component.recoveryKind()).toBe('paid');
        component.startRecoveryCheckout();
        component.continueOnFree();
        expect(TestBed.inject(CloudService).createBillingCheckoutSession).not.toHaveBeenCalled();
    });

    it('requires explicit team confirmation when the user has multiple teams', async () => {
        const component = await showRecovery(
            { checkout: 'canceled', plan: 'business', billing: 'monthly', team: billingState.team_id },
            { ...billingState, subscription_status: 'pending_checkout', pending_plan_code: 'business', billing: 'monthly' },
            true
        );
        component.startRecoveryCheckout();
        expect(TestBed.inject(CloudService).createBillingCheckoutSession).not.toHaveBeenCalled();
        component.confirmRecoveryTeam();
        const redirectSpy = vi.spyOn(component, 'redirectTo').mockImplementation(() => undefined);
        component.startRecoveryCheckout();
        expect(TestBed.inject(CloudService).createBillingCheckoutSession).toHaveBeenCalledWith({ plan_code: 'business', billing: 'monthly', locale: 'en' });
        expect(redirectSpy).toHaveBeenCalledWith('https://checkout.stripe.test/session');
    });

    it('hides usage limits and cloud plan details for OSS instances', async () => {
        fixture.destroy();
        const cloudService = TestBed.inject(CloudService);
        vi.mocked(cloudService.getPlans).mockClear();
        systemStatusResponse = {
            needs_setup: false,
            version: 'v2.0.0'
        };

        fixture = TestBed.createComponent(TeamOverviewPage);
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        const text = fixture.nativeElement.textContent;
        expect(text).not.toContain('Usage and limits');
        expect(text).not.toContain('Cloud plan');
        expect(text).toContain('Acme Analytics');
        expect(text).toContain('Members');
        expect(cloudService.getPlans).not.toHaveBeenCalled();
    });
});
