-- add_can_use_desktop_app_to_users
-- Version: 1440781686

ALTER TABLE users ADD COLUMN can_use_desktop_app bool DEFAULT NULL;
