-- Receipts deliberately have no user/tenant FK: deletion must not retain
-- operative account authority or erase the information needed to resume it.
CREATE TABLE account_deletion_intents (
    attempt_id VARCHAR PRIMARY KEY,
    user_id UUID NOT NULL,
    session_id VARCHAR NOT NULL DEFAULT '',
    consequence_hash VARCHAR NOT NULL,
    choices VARCHAR NOT NULL,
    consequences VARCHAR NOT NULL,
    cloud BOOLEAN NOT NULL,
    phase VARCHAR NOT NULL DEFAULT 'prepared',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Minimal billing/legal tombstone, never a membership or credential source.
CREATE TABLE closed_cloud_accounts (
    original_team_id UUID PRIMARY KEY,
    stripe_customer_id VARCHAR NOT NULL DEFAULT '',
    stripe_subscription_id VARCHAR NOT NULL DEFAULT '',
    cancel_at_period_end BOOLEAN NOT NULL,
    removal_started BOOLEAN NOT NULL DEFAULT FALSE,
    current_period_end TIMESTAMPTZ,
    closed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
