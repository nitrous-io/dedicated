-- create_github
-- Version: 1446020310

CREATE TABLE github (
  id bigserial PRIMARY KEY NOT NULL,
  user_id bigint REFERENCES users(id) NOT NULL,
  accesses_at character varying(255) DEFAULT NULL,
  threshold int DEFAULT 10 NOT NULL,
  created_at timestamp without time zone DEFAULT now(),
  updated_at timestamp without time zone DEFAULT now()
);
CREATE UNIQUE INDEX index_github_on_user_id ON github USING btree (user_id);
