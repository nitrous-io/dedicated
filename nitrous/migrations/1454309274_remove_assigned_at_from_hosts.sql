-- remove_assigned_at_from_hosts
-- Version: 1454309274

ALTER TABLE hosts DROP COLUMN assigned_at;
