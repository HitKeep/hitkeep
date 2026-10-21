-- Public PKCE registrations belong to the managing team; no client secrets are stored.
CREATE TABLE IF NOT EXISTS oauth_clients (
 id VARCHAR PRIMARY KEY,
 tenant_id UUID NOT NULL REFERENCES tenants(id),
 metadata VARCHAR NOT NULL,
 revision VARCHAR NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 revoked_at TIMESTAMPTZ
);

ALTER TABLE oauth_grants ADD COLUMN client_registration_revision VARCHAR DEFAULT '';
