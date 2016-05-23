-- add_git_name_and_git_email_to_user_profiles
-- Version: 1463710410

ALTER TABLE user_profiles ADD COLUMN git_name character varying(255) DEFAULT NULL;
ALTER TABLE user_profiles ADD COLUMN git_email character varying(255) DEFAULT NULL;
