-- create_quickstarts
-- Version: 1458109504
-- Please specify your SQL migration here

CREATE TABLE quickstarts (
  id bigserial PRIMARY KEY NOT NULL,
  user_id bigint REFERENCES users(id) NOT NULL,
  template_id bigint REFERENCES templates(id) NOT NULL,
  container_id bigint REFERENCES containers(id) NOT NULL,
  repo_url character varying(255),
  created_at timestamp without time zone DEFAULT now(),
  updated_at timestamp without time zone DEFAULT now()
);

CREATE INDEX index_quickstarts_on_user_id ON quickstarts USING btree (user_id);
CREATE INDEX index_quickstarts_on_template_id ON quickstarts USING btree (container_id);
CREATE INDEX index_quickstarts_on_container_id ON quickstarts USING btree (template_id);
CREATE INDEX index_quickstarts_on_repo_url ON quickstarts USING btree (repo_url);
