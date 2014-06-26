package db

var Migrations []*Migration

func init() {
	Migrations = []*Migration{
		{
			Version: "10",
			Up: `
				CREATE TABLE users (
					id serial PRIMARY KEY NOT NULL,
					username character varying(20) NOT NULL,
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
				CREATE TABLE access_tokens (
					id serial PRIMARY KEY NOT NULL,
					user_id integer NOT NULL,
					token character varying(255) NOT NULL,
					created_at timestamp without time zone DEFAULT now()
				);
				CREATE UNIQUE INDEX index_access_tokens_on_token ON access_tokens USING btree (token);
			`,
		},
	}
}
