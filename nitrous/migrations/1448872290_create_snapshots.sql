-- create_snapshots
-- Version: 1448872290

CREATE TABLE snapshots (
  id bigserial PRIMARY KEY NOT NULL,
  image_id bigint REFERENCES images(id) NOT NULL,
  user_id bigint REFERENCES users(id) NOT NULL,
  org_id bigint REFERENCES orgs(id) DEFAULT NULL,
  slug character varying(255) NOT NULL, -- .e.g. 'nitrous-io/busybox'
  namespace character varying(255) NOT NULL, -- e.g. 'nitrous-io' (org slug or username)
  name character varying(255) NOT NULL, -- e.g. 'busybox'
  tag character varying(255) NOT NULL DEFAULT 'latest', -- e.g. 'latest' or '2'
  original_image character varying(255) NOT NULL, -- Docker image that container was created from
  display_name character varying(255) NOT NULL, -- pretty name for display purposes
  size bigint DEFAULT NULL,
  created_at timestamp without time zone DEFAULT now(),
  updated_at timestamp without time zone DEFAULT now()
);

CREATE UNIQUE INDEX index_snapshots_on_image_id ON snapshots USING btree (image_id);
CREATE INDEX index_snapshots_on_user_id ON snapshots USING btree (user_id);
CREATE INDEX index_snapshots_on_org_id ON snapshots USING btree (org_id);
CREATE UNIQUE INDEX index_snapshots_on_slug_and_tag ON snapshots USING btree (slug, tag);
CREATE INDEX index_snapshots_on_slug ON snapshots USING btree (slug);

