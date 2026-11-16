ALTER TABLE account_deletion_intents ADD COLUMN providers_checked BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE account_deletion_intents ADD COLUMN provider_outcomes VARCHAR NOT NULL DEFAULT '[]';
