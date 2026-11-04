import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { INSTANCE_CAPABILITIES, SITE_CAPABILITIES, TEAM_CAPABILITIES } from '@core/access/capabilities';
import { SiteService } from '@features/sites/services/site.service';
import { PermissionService } from '@services/permission.service';
import { TeamService } from '@services/team.service';
import { AccessService } from './access.service';
import { EmbedService } from '@services/embed.service';

describe('AccessService', () => {
    let access: AccessService;
    let permissions: PermissionService;
    let sites: SiteService;
    let teams: TeamService;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [provideHttpClient(), provideHttpClientTesting()]
        });
        access = TestBed.inject(AccessService);
        permissions = TestBed.inject(PermissionService);
        sites = TestBed.inject(SiteService);
        teams = TestBed.inject(TeamService);
    });

    it('uses backend-derived instance, site, and active-team capabilities', () => {
        teams.applyTeams({
            active_team_id: 'team-1',
            teams: [{ id: 'team-1', name: 'Team 1', logo_url: '', role: 'admin', created_at: '2026-01-01T00:00:00Z' }]
        });
        permissions.applyPermissions({
            instance_role: 'admin',
            permissions: { 'site-1': 'viewer' },
            instance_permissions: ['site.view'],
            instance_capabilities: [INSTANCE_CAPABILITIES.viewSystem, SITE_CAPABILITIES.view],
            site_capabilities: { 'site-1': [SITE_CAPABILITIES.view] },
            active_team_id: 'team-1',
            active_team_role: 'admin',
            active_team_capabilities: [TEAM_CAPABILITIES.manageSettings]
        });

        expect(access.hasInstance(INSTANCE_CAPABILITIES.viewSystem)).toBe(true);
        expect(access.canSite('site-1', SITE_CAPABILITIES.view)).toBe(true);
        expect(access.canSite('site-1', SITE_CAPABILITIES.manageData)).toBe(false);
        expect(access.canActiveTeam(TEAM_CAPABILITIES.manageSettings)).toBe(true);
        expect(access.canActiveTeam(TEAM_CAPABILITIES.archive)).toBe(false);
    });

    it('never inherits private account write permissions inside an embed', () => {
        TestBed.resetTestingModule();
        TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting(), { provide: EmbedService, useValue: { isEmbedMode: () => true } }] });
        const embedded = TestBed.inject(AccessService);
        TestBed.inject(PermissionService).applyPermissions({ instance_role: 'owner', permissions: { 'site-1': 'owner' }, active_team_role: 'owner' });
        expect(embedded.hasInstance(INSTANCE_CAPABILITIES.viewSystem)).toBe(false);
        expect(embedded.canSite('site-1', SITE_CAPABILITIES.manageGoals)).toBe(false);
        expect(embedded.canActiveTeam(TEAM_CAPABILITIES.manageSettings)).toBe(false);
    });

    it('checks the active site through the shared site state', () => {
        sites.applySites([
            {
                id: 'site-1',
                user_id: 'user-1',
                domain: 'example.com',
                created_at: '2026-01-01T00:00:00Z'
            }
        ]);
        permissions.applyPermissions({
            instance_role: 'user',
            permissions: { 'site-1': 'owner' },
            instance_permissions: [],
            site_capabilities: { 'site-1': [SITE_CAPABILITIES.view, SITE_CAPABILITIES.manageData] },
            active_team_capabilities: []
        });

        expect(access.canActiveSite(SITE_CAPABILITIES.manageData)).toBe(true);
    });

    it('returns false when no access context or active site is available', () => {
        expect(access.activeTeamRole()).toBe('');
        expect(access.hasInstance(INSTANCE_CAPABILITIES.viewSystem)).toBe(false);
        expect(access.canSite('site-1', SITE_CAPABILITIES.view)).toBe(false);
        expect(access.canActiveSite(SITE_CAPABILITIES.view)).toBe(false);
        expect(access.canActiveTeam(TEAM_CAPABILITIES.viewMembers)).toBe(false);
    });

    it('treats present but empty instance capabilities as authoritative', () => {
        permissions.applyPermissions({
            instance_role: 'owner',
            permissions: {},
            instance_permissions: [INSTANCE_CAPABILITIES.manageSystem],
            instance_capabilities: []
        });

        expect(access.hasInstance(INSTANCE_CAPABILITIES.manageSystem)).toBe(false);
        expect(access.hasInstance(INSTANCE_CAPABILITIES.manageUsers)).toBe(false);
    });

    it('treats present but empty site capabilities as authoritative', () => {
        permissions.applyPermissions({
            instance_role: 'owner',
            permissions: { 'site-1': 'owner' },
            instance_permissions: [],
            instance_capabilities: [],
            site_capabilities: { 'site-1': [] }
        });

        expect(access.canSite('site-1', SITE_CAPABILITIES.view)).toBe(false);
        expect(access.canSite('site-1', SITE_CAPABILITIES.delete)).toBe(false);
    });

    it('treats present but empty matching active-team capabilities as authoritative', () => {
        teams.applyTeams({
            active_team_id: 'team-1',
            teams: [{ id: 'team-1', name: 'Team 1', logo_url: '', role: 'owner', created_at: '2026-01-01T00:00:00Z' }]
        });
        permissions.applyPermissions({
            instance_role: 'user',
            permissions: {},
            active_team_id: 'team-1',
            active_team_role: 'owner',
            active_team_capabilities: []
        });

        expect(access.canActiveTeam(TEAM_CAPABILITIES.viewMembers)).toBe(false);
        expect(access.canActiveTeam(TEAM_CAPABILITIES.archive)).toBe(false);
    });

    it('does not trust stale active-team capabilities after the active team changes', () => {
        teams.applyTeams({
            active_team_id: 'team-2',
            teams: [{ id: 'team-2', name: 'Team 2', logo_url: '', role: 'member', created_at: '2026-01-01T00:00:00Z' }]
        });
        permissions.applyPermissions({
            instance_role: 'user',
            permissions: {},
            active_team_id: 'team-1',
            active_team_role: 'owner',
            active_team_capabilities: [TEAM_CAPABILITIES.archive]
        });

        expect(access.canActiveTeam(TEAM_CAPABILITIES.archive)).toBe(false);
        expect(access.canActiveTeam(TEAM_CAPABILITIES.viewMembers)).toBe(true);
    });

    it('falls back to transitional role mappings when derived capabilities are absent', () => {
        sites.applySites([
            {
                id: 'site-owner',
                user_id: 'user-1',
                domain: 'owner.example.com',
                created_at: '2026-01-01T00:00:00Z'
            }
        ]);
        teams.applyTeams({
            active_team_id: 'team-1',
            teams: [{ id: 'team-1', name: 'Team', logo_url: '', role: 'member', created_at: '2026-01-01T00:00:00Z' }]
        });
        permissions.applyPermissions({
            instance_role: 'owner',
            permissions: {
                'site-owner': 'owner',
                'site-admin': 'admin',
                'site-editor': 'editor',
                'site-viewer': 'viewer'
            },
            instance_permissions: [],
            active_team_role: ''
        });

        expect(access.hasInstance(INSTANCE_CAPABILITIES.manageSystem)).toBe(true);
        expect(access.hasInstance('unknown.capability')).toBe(false);
        expect(access.canSite('site-owner', SITE_CAPABILITIES.delete)).toBe(true);
        expect(access.canSite('site-editor', SITE_CAPABILITIES.manageData)).toBe(true);
        expect(access.activeTeamRole()).toBe('');

        permissions.applyPermissions({
            instance_role: 'user',
            permissions: {
                'site-owner': 'owner',
                'site-admin': 'admin',
                'site-editor': 'editor',
                'site-viewer': 'viewer'
            },
            instance_permissions: [],
            active_team_role: ''
        });

        permissions.applyPermissions({
            instance_role: 'user',
            permissions: {
                'site-owner': 'owner',
                'site-admin': 'admin',
                'site-editor': 'editor',
                'site-viewer': 'viewer',
                'site-unknown': 'unknown' as never
            },
            instance_permissions: [],
            active_team_role: 'owner'
        });

        expect(access.activeTeamRole()).toBe('owner');
        expect(access.canActiveTeam(TEAM_CAPABILITIES.archive)).toBe(true);
        expect(access.canSite('site-owner', SITE_CAPABILITIES.delete)).toBe(true);
        expect(access.canSite('site-unknown', SITE_CAPABILITIES.view)).toBe(false);

        permissions.applyPermissions({
            instance_role: 'user',
            permissions: {
                'site-owner': 'owner',
                'site-admin': 'admin',
                'site-editor': 'editor',
                'site-viewer': 'viewer'
            },
            instance_permissions: [],
            active_team_role: ''
        });

        expect(access.canSite('site-admin', SITE_CAPABILITIES.manageTeam)).toBe(true);
        expect(access.canSite('site-editor', SITE_CAPABILITIES.manageGoals)).toBe(true);
        expect(access.canSite('site-editor', SITE_CAPABILITIES.manageData)).toBe(false);
        expect(access.canSite('site-viewer', SITE_CAPABILITIES.view)).toBe(true);
        expect(access.canActiveTeam(TEAM_CAPABILITIES.viewMembers)).toBe(true);
        expect(access.canActiveTeam(TEAM_CAPABILITIES.manageMembers)).toBe(false);
    });
});
