CREATE TABLE social_identities_apple (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id UUID NOT NULL REFERENCES users (id),
    provider VARCHAR NOT NULL CHECK (provider IN ('google', 'github', 'microsoft', 'apple')),
    subject VARCHAR NOT NULL,
    observed_email VARCHAR NOT NULL DEFAULT '',
    linked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    apple_client_id VARCHAR NOT NULL DEFAULT '',
    apple_refresh_token_ciphertext VARCHAR NOT NULL DEFAULT '',
    UNIQUE (provider, subject),
    UNIQUE (user_id, provider)
);

INSERT INTO social_identities_apple (id,user_id,provider,subject,observed_email,linked_at,updated_at,last_used_at)
SELECT id,user_id,provider,subject,observed_email,linked_at,updated_at,last_used_at FROM social_identities;
DROP TABLE social_identities;
ALTER TABLE social_identities_apple RENAME TO social_identities;
CREATE INDEX social_identities_user_idx ON social_identities (user_id);

CREATE TABLE pending_social_confirmations_apple (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    token_hash VARCHAR UNIQUE NOT NULL,
    provider VARCHAR NOT NULL CHECK (provider IN ('google', 'github', 'microsoft', 'apple')),
    subject VARCHAR NOT NULL,
    observed_email VARCHAR NOT NULL DEFAULT '',
    target_email VARCHAR NOT NULL,
    target_user_id UUID REFERENCES users (id),
    team_name VARCHAR NOT NULL DEFAULT '',
    jurisdiction VARCHAR NOT NULL DEFAULT '',
    locale VARCHAR NOT NULL DEFAULT '',
    plan_code VARCHAR NOT NULL DEFAULT 'free',
    billing_interval VARCHAR NOT NULL DEFAULT 'monthly',
    accepted_tos_at TIMESTAMPTZ,
    return_path VARCHAR NOT NULL DEFAULT '/dashboard',
    remember_me BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, subject)
);

INSERT INTO pending_social_confirmations_apple SELECT * FROM pending_social_confirmations;
DROP TABLE pending_social_confirmations;
ALTER TABLE pending_social_confirmations_apple RENAME TO pending_social_confirmations;
CREATE INDEX pending_social_confirmations_expires_idx ON pending_social_confirmations (expires_at);
CREATE INDEX pending_social_confirmations_user_idx ON pending_social_confirmations (target_user_id);
