package db

var Migrations []*Migration

func init() {
	Migrations = []*Migration{
		{
			Version: "10",
			Up: `
				CREATE TABLE users (
					id bigserial PRIMARY KEY NOT NULL,
					username character varying(30) NOT NULL,
					email character varying(255) NOT NULL,
					encrypted_password character varying(255) NOT NULL,
					name character varying(255) DEFAULT '' NOT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE UNIQUE INDEX index_users_on_username ON users USING btree (username);
				CREATE UNIQUE INDEX index_users_on_email ON users USING btree (email);
			`,
		},
		{
			Version: "20",
			Up: `
				CREATE TABLE tokens (
					id bigserial PRIMARY KEY NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					token character varying(255) NOT NULL,
					created_at timestamp without time zone DEFAULT now()
				);
				CREATE UNIQUE INDEX index_tokens_on_token ON tokens USING btree (token);
			`,
		},
		{
			Version: "30",
			Up: `
				CREATE TABLE regions (
					id bigserial PRIMARY KEY NOT NULL,
					name character varying(255) NOT NULL,
					description character varying(255) NOT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE UNIQUE INDEX index_regions_on_name ON regions USING btree (name);
			`,
		},
		{
			Version: "40",
			Up: `
				CREATE TABLE plans (
					id bigserial PRIMARY KEY NOT NULL,
					name character varying(255) NOT NULL,
					max_containers bigint NOT NULL,
					price bigint NOT NULL, -- Price in cents so that we always deal with integers.
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE UNIQUE INDEX index_plans_on_name ON plans USING btree (name);
			`,
		},
		{
			Version: "50",
			Up: `
				CREATE TABLE provider_templates (
					id bigserial PRIMARY KEY NOT NULL,
					region_id bigint REFERENCES regions(id) NOT NULL,
					plan_id bigint REFERENCES plans(id) NOT NULL,
					name character varying(255) NOT NULL,
					provider character varying(255) NOT NULL,
					config hstore,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_provider_templates_on_region_id ON provider_templates USING btree (region_id);
				CREATE INDEX index_provider_templates_on_plan_id ON provider_templates USING btree (plan_id);
			`,
		},
	}
}
