-- add_createuser_to_oauth_clients
-- Version: 1441821418

ALTER TABLE oauth_clients ADD COLUMN create_user bool DEFAULT false;
ALTER TABLE users ADD COLUMN created_by_client_id bigint REFERENCES oauth_clients(id) DEFAULT NULL;