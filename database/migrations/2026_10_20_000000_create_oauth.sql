CREATE TABLE IF NOT EXISTS oauth_grants (
 id UUID PRIMARY KEY,
 user_id UUID NOT NULL,
 tenant_id UUID NOT NULL REFERENCES tenants(id),
 client_id VARCHAR NOT NULL,
 client_name VARCHAR NOT NULL,
 client_uri VARCHAR NOT NULL DEFAULT '',
 audience VARCHAR NOT NULL,
 scopes VARCHAR NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL,
 last_used_at TIMESTAMPTZ,
 revoked_at TIMESTAMPTZ
);

-- Credential children carry their tenant scope; grant existence is checked in every operation.
CREATE TABLE IF NOT EXISTS oauth_credentials (
 kind VARCHAR NOT NULL CHECK (kind IN ('code', 'access', 'refresh', 'pkce')),
 signature VARCHAR NOT NULL,
 oauth_grant_id UUID NOT NULL,
 tenant_id UUID NOT NULL REFERENCES tenants(id),
 payload VARCHAR NOT NULL,
 access_signature VARCHAR NOT NULL DEFAULT '',
 expires_at TIMESTAMPTZ NOT NULL,
 consumed_at TIMESTAMPTZ,
 revoked_at TIMESTAMPTZ,
 PRIMARY KEY(kind, signature)
);

CREATE TABLE IF NOT EXISTS oauth_grant_sites (
 oauth_grant_id UUID NOT NULL,
 tenant_id UUID NOT NULL REFERENCES tenants(id),
 site_id UUID NOT NULL REFERENCES sites(id),
 PRIMARY KEY(oauth_grant_id, site_id)
);

-- Browser flows exist before an account/team is selected and expire after fifteen minutes.
CREATE TABLE IF NOT EXISTS oauth_pending (
 signature VARCHAR PRIMARY KEY,
 browser_signature VARCHAR NOT NULL,
 payload VARCHAR NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL
);
