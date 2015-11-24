-- add_max_collaborators_to_plans
-- Version: 1447997154

ALTER TABLE plans ADD COLUMN max_collaborators bigint DEFAULT 1;
UPDATE plans SET max_collaborators=NULL WHERE org=true;
