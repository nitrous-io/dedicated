-- add_published_at_to_snapshots
-- Version: 1455138709

ALTER TABLE snapshots ADD COLUMN published_at timestamp without time zone DEFAULT NULL;
