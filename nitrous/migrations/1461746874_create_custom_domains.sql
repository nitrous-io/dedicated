-- create_custom_domains
-- Version: 1461746874

CREATE TABLE custom_domains (
  id bigserial PRIMARY KEY NOT NULL,
  container_id bigint REFERENCES containers(id) NOT NULL,
  domain character varying(255) NOT NULL,
  port bigint NOT NULL,
  container_port bigint NOT NULL,
  created_at timestamp without time zone DEFAULT now(),
  updated_at timestamp without time zone DEFAULT now()
);

CREATE INDEX index_custom_domains_on_container_id ON custom_domains USING btree (container_id);
CREATE INDEX index_custom_domains_on_domain ON custom_domains USING btree (domain);
CREATE UNIQUE INDEX index_custom_domains_on_domain_and_port ON custom_domains USING btree (domain, port);
