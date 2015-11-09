-- drop-segments
-- Version: 1447095657
ALTER TABLE users DROP COLUMN IF EXISTS segment_id;
ALTER TABLE orgs DROP COLUMN IF EXISTS segment_id;

DROP TABLE IF EXISTS plan_segments;
DROP TABLE IF EXISTS segments;
