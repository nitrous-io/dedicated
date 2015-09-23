-- add_segments
-- Version: 1442993803

CREATE TABLE segments (
	id bigserial PRIMARY KEY NOT NULL,
	created_at timestamp without time zone DEFAULT now(),
	updated_at timestamp without time zone DEFAULT now(),
	slug character varying(255) NOT NULL,
	is_default boolean DEFAULT false NOT NULL
);

CREATE UNIQUE INDEX index_segments_on_slug ON segments USING btree (slug);

-- enforce that only one segment can be default
CREATE UNIQUE INDEX index_segments_on_is_default ON segments USING btree (is_default) WHERE is_default IS TRUE;

-- insert default segment, unpopulated for now
INSERT INTO segments (slug, is_default) VALUES ('default', true);

ALTER TABLE users ADD COLUMN segment_id bigint REFERENCES segments(id) NULL;
UPDATE users SET segment_id = (SELECT id FROM segments WHERE is_default IS TRUE);
ALTER TABLE users ALTER COLUMN segment_id SET NOT NULL;

ALTER TABLE orgs ADD COLUMN segment_id bigint REFERENCES segments(id) NULL;
UPDATE orgs SET segment_id = (SELECT id FROM segments WHERE is_default IS TRUE);
ALTER TABLE orgs ALTER COLUMN segment_id SET NOT NULL;

CREATE TABLE plan_segments (
	id bigserial PRIMARY KEY NOT NULL,
	segment_id bigint REFERENCES segments(id) NOT NULL,
	plan_id bigint REFERENCES plans(id) NOT NULL
);

CREATE UNIQUE INDEX index_plan_segments_on_plan_id_and_segment_id ON plan_segments USING btree(segment_id, plan_id);
CREATE INDEX index_plan_segments_on_segment_id ON plan_segments USING btree(segment_id);
CREATE INDEX index_plan_segments_on_plan_id ON plan_segments USING btree(plan_id);
