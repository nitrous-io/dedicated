-- add_index_to_github_user_name
-- Version: 1443767319

CREATE INDEX index_user_profiles_on_github_user_name ON user_profiles USING btree (github_user_name);

