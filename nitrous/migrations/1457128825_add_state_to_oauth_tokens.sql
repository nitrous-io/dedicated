-- add_state_to_oauth_tokens
-- Version: 1457128825

ALTER TABLE oauth_tokens ADD COLUMN state character varying(255) DEFAULT NULL;
