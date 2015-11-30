-- Null_MaxHosts_On_Orgs
-- Version: 1448907987

ALTER TABLE orgs ALTER COLUMN max_hosts DROP NOT NULL;
ALTER TABLE orgs ALTER COLUMN max_hosts SET DEFAULT NULL;
-- update any plans created after multi host plans (created_at of plan '20151124-org-multihost-nano')
UPDATE orgs SET max_hosts = NULL WHERE created_at >= '2015-11-25 19:27:00.189261';
