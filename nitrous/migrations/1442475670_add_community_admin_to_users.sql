-- add_community_admin_to_users
-- Version: 1442475670

ALTER TABLE users ADD COLUMN community_admin bool DEFAULT false NOT NULL;
