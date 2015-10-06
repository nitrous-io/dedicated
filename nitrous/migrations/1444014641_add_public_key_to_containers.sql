-- add_public_key_to_containers
-- Version: 1444014641

ALTER TABLE containers ADD COLUMN public_key_fingerprint varchar(255) DEFAULT NULL;
