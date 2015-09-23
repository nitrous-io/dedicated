-- add_github_user_scopes_json_to_userprofiles
-- Version: 1442805478

ALTER TABLE user_profiles ADD COLUMN github_user_scopes_json character varying(255) DEFAULT NULL;
