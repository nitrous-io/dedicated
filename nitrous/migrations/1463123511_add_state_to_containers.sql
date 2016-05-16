-- add_state_to_containers
-- Version: 1463123511

ALTER TABLE containers ADD COLUMN state character varying(255) NOT NULL DEFAULT 'unknown';
