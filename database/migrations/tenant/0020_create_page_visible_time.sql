CREATE TABLE IF NOT EXISTS page_visible_time (
    site_id UUID NOT NULL,
    page_id UUID NOT NULL,
    visible_ms INTEGER NOT NULL CHECK (visible_ms BETWEEN 0 AND 86400000),
    PRIMARY KEY (site_id, page_id)
);
