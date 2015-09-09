-- add_allowed_org_slugs_to_oauth_clients
-- Version: 1441831870

ALTER TABLE oauth_clients ADD COLUMN allowed_org_slugs character varying(255) DEFAULT '';
