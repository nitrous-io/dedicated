-- add_free_user_default_to_segments.sql
-- Version: 1445414893

ALTER TABLE segments ADD COLUMN free_user_default boolean DEFAULT false NOT NULL;
CREATE UNIQUE INDEX index_segments_on_free_user_default ON segments USING btree (free_user_default) WHERE free_user_default = true;
INSERT INTO segments (slug, free_user_default) VALUES ('free-user-default', true);
