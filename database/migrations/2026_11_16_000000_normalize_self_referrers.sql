-- Classify internal navigation at query time, including stored/imported history.
CREATE OR REPLACE MACRO hk_external_referrer(ref, ref_site_id) AS (
    CASE
        WHEN regexp_replace(
            regexp_extract(lower(trim(ref)), '^(?:https?://|//)?([^/:?#]+)(?::[0-9]+)?(?:[/?#]|$)', 1),
            '^www\.', ''
        ) = (
            SELECT regexp_replace(lower(trim(domain)), '^www\.', '')
            FROM sites WHERE id = ref_site_id
        ) THEN NULL
        ELSE hk_referrer(ref)
    END
);
