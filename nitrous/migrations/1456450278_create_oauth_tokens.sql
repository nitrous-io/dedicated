-- create_oauth_tokens
-- Version: 1456450278

CREATE TABLE oauth_tokens (
  id bigserial PRIMARY KEY NOT NULL,
  user_id bigint REFERENCES users(id) NOT NULL,
  service character varying(255) NOT NULL, -- external oauth service, eg. twitter, facebook
  nitrous_token character varying(255),
  request_token character varying(255),
  access_token character varying(255),
  secret character varying(255),
  created_at timestamp without time zone DEFAULT now(),
  updated_at timestamp without time zone DEFAULT now()
);

CREATE UNIQUE INDEX index_oauth_tokens_on_user_id_and_service ON oauth_tokens USING btree (user_id, service);
