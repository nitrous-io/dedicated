-- add_downloads_snapshots
-- Version: 1450069028
ALTER TABLE snapshots ADD COLUMN downloads bigint NOT NULL DEFAULT 0;

