export interface OAuthSite {
    id: string;
    domain: string;
    name?: string;
}

export interface OAuthClientSummary {
    id: string;
    name: string;
}

export interface OAuthConsent {
    client: OAuthClientSummary & { callback_origin: string };
    resource: string;
    scopes: string[];
    expires_at: string;
    csrf_token: string;
    teams: { id: string; name: string; sites: OAuthSite[] }[];
}

export interface OAuthGrant {
    id: string;
    client: OAuthClientSummary;
    team: { id: string; name: string };
    sites: OAuthSite[];
    scopes: string[];
    resource: string;
    created_at: string;
    expires_at: string;
    last_used_at?: string | null;
}
