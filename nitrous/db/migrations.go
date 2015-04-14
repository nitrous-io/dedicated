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
		{
			Version: "60",
			Up: `
				CREATE TABLE hosts (
					id bigserial PRIMARY KEY NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					region_id bigint REFERENCES regions(id) NOT NULL,
					plan_id bigint REFERENCES plans(id) NOT NULL,
					provider_template_id bigint REFERENCES provider_templates(id) NOT NULL,
					name character varying(63) NOT NULL, -- Used as DNS label, see http://tools.ietf.org/html/rfc1034#section-3.1.
					long_name character varying(255),
					state character varying(255) NOT NULL DEFAULT 'born',
					ip_address character varying(45), -- See http://tools.ietf.org/html/rfc4291.
					provider_identifier character varying(255),
					max_containers bigint,
					terminated_at timestamp without time zone,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_hosts_on_user_id ON hosts USING btree (user_id);
				CREATE INDEX index_hosts_on_region_id ON hosts USING btree (region_id);
				CREATE UNIQUE INDEX index_hosts_on_user_id_and_name ON hosts USING btree (user_id, name);
			`,
		},
		{
			Version: "70",
			Up: `
				CREATE TABLE containers (
					id character varying(255) PRIMARY KEY NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					host_id bigint REFERENCES hosts(id) NOT NULL,
					image character varying(255) NOT NULL,
					name character varying(255) NOT NULL,
					long_name character varying(255),
					ip_address character varying(45),
					terminated_at timestamp without time zone,
					touched_at timestamp without time zone,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_containers_on_user_id ON containers USING btree (user_id);
				CREATE UNIQUE INDEX index_containers_on_host_id_and_name ON containers USING btree (host_id, name);
			`,
		},
		{
			Version: "80",
			Up: `
				DROP INDEX index_hosts_on_user_id_and_name;
				CREATE UNIQUE INDEX index_hosts_on_name ON hosts USING btree (name);
			`,
		},
		{
			Version: "90",
			Up: `
				ALTER TABLE hosts ADD COLUMN freighterd_port bigint;
			`,
		},
		{
			Version: "100",
			Up: `
				DROP INDEX index_containers_on_host_id_and_name;
				CREATE UNIQUE INDEX index_containers_on_name ON containers USING btree (name);
			`,
		},
		{
			Version: "110",
			Up: `
				ALTER TABLE containers DROP CONSTRAINT containers_pkey;
				ALTER TABLE containers RENAME COLUMN id TO cid;
				ALTER TABLE containers ADD COLUMN id bigserial PRIMARY KEY NOT NULL;
				CREATE INDEX index_containers_on_cid ON containers USING btree (cid);
			`,
		},
		{
			Version: "120",
			Up: `
				ALTER TABLE containers RENAME COLUMN name TO slug;
				ALTER TABLE containers RENAME COLUMN long_name TO name;
				DROP INDEX index_containers_on_name;
				CREATE UNIQUE INDEX index_containers_on_slug ON containers USING btree (slug);

				ALTER TABLE hosts RENAME COLUMN name TO slug;
				ALTER TABLE hosts RENAME COLUMN long_name TO name;
				DROP INDEX index_hosts_on_name;
				CREATE UNIQUE INDEX index_hosts_on_slug ON hosts USING btree (slug);
			`,
		},
		{
			Version: "130",
			Up: `
				ALTER TABLE hosts ADD COLUMN size character varying(255);
			`,
		},
		{
			Version: "140",
			Up: `
				CREATE TABLE key_pairs (
					id bigserial PRIMARY KEY NOT NULL,
					host_id bigint,
					public_key text NOT NULL,
					encrypted_private_key text NOT NULL,
					encrypted_private_key_iv character varying(255) NOT NULL,
					terminated_at timestamp without time zone,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_key_pairs_on_host_id ON key_pairs USING btree (host_id);
			`,
		},
		{
			Version: "150",
			Up: `
				CREATE TABLE public_keys (
					id bigserial PRIMARY KEY NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					container_id bigserial REFERENCES containers(id) NOT NULL,
					key text NOT NULL,
					fingerprint character varying(255) NOT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_public_keys_on_user_id ON public_keys USING btree (user_id);
				CREATE UNIQUE INDEX index_public_keys_on_container_id_and_key ON public_keys USING btree (container_id, key);
			`,
		},
		{
			Version: "160",
			Up: `
				CREATE TABLE volumes (
					id bigserial PRIMARY KEY NOT NULL,
					host_id bigint REFERENCES hosts(id) NOT NULL,
					name character varying(255) NOT NULL,
					slug character varying(255) NOT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_volumes_on_host_id ON volumes USING btree (host_id);
				CREATE UNIQUE INDEX index_volumes_on_host_id_and_slug ON volumes USING btree (host_id, slug);
			`,
		},
		{
			Version: "170",
			Up: `
				CREATE TABLE registries (
					id bigserial PRIMARY KEY NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					host_id bigint REFERENCES hosts(id) NOT NULL,
					url character varying(45),
					username character varying(45),
					email character varying(45),
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_registries_on_user_id ON registries USING btree (user_id);
				CREATE UNIQUE INDEX index_registries_on_host_id_and_url ON registries USING btree (host_id, url);
			`,
		},
		{
			Version: "180",
			Up: `
				CREATE TABLE certs (
					id bigserial PRIMARY KEY NOT NULL,
					host_id bigint,
					ca text NOT NULL,
					server_cert text NOT NULL,
					server_key text NOT NULL,
					client_cert text NOT NULL,
					client_key text NOT NULL,
					terminated_at timestamp without time zone,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_certs_on_host_id ON certs USING btree (host_id);
			`,
		},
		{
			Version: "190",
			Up: `
				CREATE TABLE container_volumes (
					id bigserial PRIMARY KEY NOT NULL,
					container_id bigint REFERENCES containers(id) NOT NULL,
					volume_id bigint REFERENCES volumes(id) NOT NULL,
					created_at timestamp without time zone DEFAULT now()
				);
				CREATE UNIQUE INDEX index_container_volumes_on_container_id_and_volume_id ON container_volumes USING btree (container_id, volume_id);
			`,
		},
		{
			Version: "200",
			Up: `
				ALTER TABLE containers DROP COLUMN ip_address;
			`,
		},
		{
			Version: "210",
			Up: `
				DROP TABLE certs;
			`,
		},
		{
			Version: "215",
			Up: `
				DROP INDEX index_plans_on_name;
        ALTER TABLE plans ADD COLUMN slug character varying(255);
        UPDATE plans SET slug=name;
        ALTER TABLE plans ALTER COLUMN slug SET NOT NULL;
				CREATE UNIQUE INDEX index_plans_on_slug ON plans USING btree (slug);
        DROP INDEX index_regions_on_name;
				ALTER TABLE regions RENAME COLUMN name TO slug;
				CREATE UNIQUE INDEX index_regions_on_slug ON regions USING btree (slug);
				ALTER TABLE regions RENAME COLUMN description TO name;
      `,
		},
		{
			Version: "220",
			Up: `
				ALTER TABLE tokens ALTER COLUMN token SET DEFAULT left(encode(gen_random_bytes(191), 'base64'), 255);
				ALTER TABLE hosts ADD COLUMN secret character varying(255) NOT NULL DEFAULT left(encode(gen_random_bytes(191), 'base64'), 255);
			`,
		},
		{
			Version: "225",
			Up: `
				ALTER TABLE regions ADD COLUMN rank bigint NOT NULL DEFAULT 0;
				ALTER TABLE plans ADD COLUMN cpu character varying(255) NOT NULL DEFAULT '';
				ALTER TABLE plans ADD COLUMN memory character varying(255) NOT NULL DEFAULT '';
				ALTER TABLE plans ADD COLUMN storage character varying(255) NOT NULL DEFAULT '';
			`,
		},
		{
			Version: "230",
			Up: `
				ALTER TABLE containers ADD COLUMN secret character varying(255) NOT NULL DEFAULT left(encode(gen_random_bytes(191), 'base64'), 255);
			`,
		},
		{
			Version: "240",
			Up: `
				ALTER TABLE containers ADD COLUMN locked_at timestamp without time zone;
			`,
		},
		{
			Version: "250",
			Up: `
				CREATE TABLE images (
					id bigserial PRIMARY KEY NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					host_id bigint REFERENCES hosts(id) NOT NULL,
					freighter_id character varying(255) NOT NULL,
					name character varying(255),
					tag character varying(255),
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_images_on_user_id ON images USING btree (user_id);
				CREATE INDEX index_images_on_host_id ON images USING btree (host_id);
				CREATE INDEX index_images_on_freighter_id ON images USING btree (freighter_id);
			`,
		},
		{
			Version: "260",
			Up: `
				ALTER TABLE images ADD COLUMN slug character varying(255);
				CREATE INDEX index_images_on_slug_and_tag ON images USING btree (slug, tag);
			`,
		},
		{
			Version: "270",
			Up: `
				CREATE TABLE base_images (
					id bigserial PRIMARY KEY NOT NULL,
					name character varying(255) NOT NULL,
					repo character varying(255) NOT NULL,
					description character varying(255) NOT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE UNIQUE INDEX index_base_images_on_name ON base_images USING btree (name);
			`,
		},
		{
			Version: "280",
			Up: `
				CREATE TABLE certs (
					id bigserial PRIMARY KEY NOT NULL,
					host_id bigint,
					ca text NOT NULL,
					server_cert text NOT NULL,
					server_key text NOT NULL,
					terminated_at timestamp without time zone,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_certs_on_host_id ON certs USING btree (host_id);
			`,
		},
		{
			Version: "285",
			Up: `
				ALTER TABLE certs DROP COLUMN ca;
			`,
		},
		{
			Version: "300",
			Up: `
				CREATE TABLE user_profiles (
					id bigserial PRIMARY KEY NOT NULL,
					max_hosts bigint NOT NULL DEFAULT 1,
					user_id bigint REFERENCES users(id) NOT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE UNIQUE INDEX index_user_profiles_on_user_id ON user_profiles USING btree (user_id);
			`,
		},

		// Note: existing users will be limited to one host by default
		// If you have a test account, change this value manually
		{
			Version: "310",
			Up: `
				INSERT INTO user_profiles (user_id) (
					SELECT users.id FROM users WHERE NOT EXISTS (SELECT user_id FROM user_profiles)
				);
			`,
		},
		{
			Version: "315",
			Up: `
				ALTER TABLE base_images ADD COLUMN slug character varying(255) DEFAULT '' NOT NULL;
			`,
		},
		{
			Version: "320",
			Up: `
				ALTER TABLE volumes ADD COLUMN system boolean DEFAULT false NOT NULL;
			`,
		},
		{
			Version: "330",
			Up: `
				ALTER TABLE users ADD COLUMN login_attempts smallint DEFAULT 0 NOT NULL ;
			`,
		},
		{
			Version: "340",
			Up: `
				CREATE TABLE invite_codes (
					id bigserial PRIMARY KEY NOT NULL,
					code character varying(32) NOT NULL,
					used bool DEFAULT false NOT NULL
				);
				CREATE UNIQUE INDEX index_invite_codes_on_code ON invite_codes USING btree (code);
			`,
		},
		{
			Version: "345",
			Up: `
				ALTER TABLE users ADD COLUMN invite_code_id bigint REFERENCES invite_codes(id);
			`,
		},
		{
			Version: "350",
			Up: `
				ALTER TABLE users ADD COLUMN admin boolean DEFAULT false NOT NULL;
			`,
		},
		{
			Version: "360",
			Up: `
				ALTER TABLE plans ADD COLUMN disabled boolean DEFAULT false NOT NULL;
			`,
		},
		{
			Version: "370",
			Up: `
				ALTER TABLE user_profiles ADD COLUMN advanced boolean DEFAULT false NOT NULL;
			`,
		},
		{
			Version: "380",
			Up: `
				ALTER TABLE users ADD COLUMN reset_password_token character varying(255);
				ALTER TABLE users ADD COLUMN reset_password_sent_at timestamp without time zone;
			`,
		},
		{
			Version: "390",
			Up: `
				ALTER TABLE hosts ADD COLUMN preprovisioned boolean DEFAULT false NOT NULL;
			`,
		},
		{
			Version: "395",
			Up: `
				ALTER TABLE provider_templates
				  ADD COLUMN preprovision_max smallint
				    CONSTRAINT cost_control CHECK (preprovision_max < 10)
				    DEFAULT 0 NOT NULL;
			`,
		},
		{
			Version: "400",
			Up: `
				ALTER TABLE users ADD COLUMN confirmation_token character varying(255) DEFAULT NULL;
				ALTER TABLE users ADD COLUMN confirmation_sent_at timestamp without time zone DEFAULT NULL;
				ALTER TABLE users ADD COLUMN confirmed_at timestamp without time zone DEFAULT NULL;
				CREATE UNIQUE INDEX index_users_on_confirmation_token ON users USING btree (confirmation_token);
				UPDATE users SET confirmed_at = now();
				`,
		},
		{
			Version: "410",
			Up: `
				CREATE TYPE billing_state AS ENUM ('free', 'paid', 'payment_due', 'delinquent');
				CREATE TABLE billing_profiles (
					id bigserial PRIMARY KEY NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					stripe_id character varying(32),
					state billing_state NOT NULL DEFAULT 'free',
					last_four character varying(32),
					card_type character varying(32),
					exp_month integer,
					exp_year integer,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE UNIQUE INDEX index_billing_profiles_on_user_id ON billing_profiles USING btree (user_id);
				CREATE UNIQUE INDEX index_billing_profiles_on_stripe_id ON billing_profiles USING btree (stripe_id);
			`,
		},
		{
			Version: "415",
			Up: `
				ALTER TABLE hosts ADD COLUMN assigned_at timestamp without time zone;
			`,
		},
		{
			Version: "420",
			Up: `
				ALTER TABLE users ADD COLUMN billing_enabled boolean DEFAULT false NOT NULL;
				UPDATE users SET billing_enabled = admin;
			`,
		},
		{
			Version: "430",
			Up: `
				INSERT INTO billing_profiles (user_id) (
					SELECT users.id FROM users WHERE NOT EXISTS (SELECT user_id FROM billing_profiles)
				);
			`,
		},
		{
			Version: "440",
			Up: `
				ALTER TABLE users ADD COLUMN unconfirmed_email character varying(255) DEFAULT NULL
			`,
		},
		{
			Version: "450",
			Up: `
				CREATE TABLE host_transitions (
					id bigserial PRIMARY KEY NOT NULL,
					host_id bigint REFERENCES hosts(id) NOT NULL,
					previous_state character varying(255),
					state character varying(255) NOT NULL,
					created_at timestamp without time zone DEFAULT now(),
					ended_at timestamp without time zone DEFAULT NULL
				);
				CREATE INDEX index_host_transitions_on_host_id ON host_transitions USING btree (host_id);
			`,
		},
		{
			Version: "460",
			Up: `
				CREATE TABLE templates (
					id bigserial PRIMARY KEY NOT NULL,
					slug character varying(255) NOT NULL,
					name character varying(255),
					description character varying,
					config character varying NOT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE UNIQUE INDEX index_templates_on_slug ON templates USING btree (slug);
			`,
		},
		{
			Version: "470",
			Up: `
				CREATE TABLE host_usages (
					id bigserial PRIMARY KEY NOT NULL,
					host_id bigint REFERENCES hosts(id) NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					hours int NOT NULL,
					day date NOT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_host_usages_on_user_id_and_day ON host_usages USING btree (user_id, day);
			`,
		},
		{
			Version: "480",
			Up: `
				ALTER TABLE hosts ADD COLUMN hourly_price bigint NOT NULL DEFAULT 1;
				ALTER TABLE provider_templates ADD COLUMN hourly_price bigint NOT NULL DEFAULT 1;
			`,
		},
		{
			Version: "490",
			Up: `
				CREATE TABLE invoices (
					id bigserial PRIMARY KEY NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					total_charge bigint NOT NULL DEFAULT 0,
					period_start timestamp without time zone NOT NULL,
					period_end timestamp without time zone NOT NULL,
					successful_charge boolean DEFAULT FALSE,
					last_attempt_at timestamp without time zone DEFAULT NULL,
					first_attempt_at timestamp without time zone DEFAULT NULL,
					attempts int DEFAULT 0,
					stripe_id character varying(255) DEFAULT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_invoices_on_user_id ON invoices USING btree (user_id);

				CREATE TABLE invoice_items (
					id bigserial PRIMARY KEY NOT NULL,
					invoice_id bigint REFERENCES invoices(id) NOT NULL,
					host_id bigint REFERENCES hosts(id) NOT NULL,
					hours int NOT NULL,
					hourly_price bigint NOT NULL,
					stripe_id character varying(255) DEFAULT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
 				);
				CREATE INDEX index_invoice_items_on_invoice_id ON invoice_items USING btree (invoice_id);
			`,
		},
		{
			Version: "500",
			Up: `
				ALTER TABLE invoices ADD COLUMN failed boolean DEFAULT FALSE;
				CREATE UNIQUE INDEX index_invoices_on_stripe_id ON invoices USING btree (stripe_id);
			`,
		},
		{
			Version: "510",
			Up: `
				ALTER TABLE billing_profiles ADD COLUMN stripe_fingerprint character varying(255);
				CREATE INDEX index_billing_profiles_on_stripe_fingerprint ON billing_profiles USING btree (stripe_fingerprint);
			`,
		},
		{
			Version: "520",
			Up: `
				ALTER TABLE invoices ADD COLUMN deferred boolean DEFAULT FALSE;
				ALTER TABLE invoices ADD COLUMN covered_by bigint REFERENCES invoices(id);
				CREATE INDEX index_invoices_on_deferred ON invoices USING btree (deferred);
				CREATE INDEX index_invoices_on_covered_by ON invoices USING btree (covered_by);
				CREATE UNIQUE INDEX index_invoice_items_on_stripe_id ON invoice_items USING btree (stripe_id);
			`,
		},
		{
			Version: "530",
			Up: `
				ALTER TABLE hosts ADD COLUMN freighterd_version character varying(255) DEFAULT '' NOT NULL;
				ALTER TABLE hosts ADD COLUMN docker_version character varying(255) DEFAULT '' NOT NULL;
			`,
		},
		{
			Version: "540",
			Up: `
				CREATE TABLE credit_types (
					id bigserial PRIMARY KEY NOT NULL,
					name character varying(255),
					description character varying,
					active boolean DEFAULT FALSE,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);

				CREATE TABLE credits (
					id bigserial PRIMARY KEY NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					invoice_id bigint REFERENCES invoices(id) DEFAULT NULL,
					credit_type_id bigint REFERENCES credit_types(id) DEFAULT NULL,
					amount bigint NOT NULL DEFAULT 0,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE INDEX index_credits_on_user_id ON credits USING btree (user_id);
			`,
		},
		{
			Version: "550",
			Up: `
				ALTER TABLE invoices ADD COLUMN credit_amount bigint DEFAULT 0;
				ALTER TABLE invoice_items ADD COLUMN credit bigint DEFAULT 0;
				ALTER TABLE invoice_items ALTER COLUMN hours DROP NOT NULL;
				ALTER TABLE invoice_items ALTER COLUMN hourly_price DROP NOT NULL;
				ALTER TABLE invoice_items ALTER COLUMN host_id SET DEFAULT NULL;
				ALTER TABLE invoice_items DROP CONSTRAINT invoice_items_host_id_fkey;
				`,
		},
		{
			Version: "560",
			Up: `
				ALTER TABLE hosts ALTER COLUMN freighterd_version DROP DEFAULT;
				ALTER TABLE hosts ALTER COLUMN docker_version DROP DEFAULT;
			`,
		},
		{
			Version: "570",
			Up: `
				ALTER TABLE invoice_items ALTER COLUMN host_id DROP NOT NULL;
				ALTER TABLE invoice_items ALTER COLUMN credit SET DEFAULT NULL;
				ALTER TABLE invoice_items ALTER COLUMN hours TYPE bigint;
				`,
		},
		{
			Version: "580",
			Up: `
				CREATE TABLE email_captures (
					id bigserial PRIMARY KEY NOT NULL,
					email_address character varying,
					created_at timestamp without time zone DEFAULT now()
				);
			`,
		},
		{
			Version: "590",
			Up: `
				ALTER TABLE users ALTER COLUMN billing_enabled SET DEFAULT TRUE;
				ALTER TABLE user_profiles ALTER COLUMN max_hosts SET DEFAULT 3;
			`,
		},
		{
			Version: "600",
			Up: `
				ALTER TABLE billing_profiles ADD COLUMN pending_activation boolean DEFAULT TRUE;
				ALTER TABLE credit_types ADD COLUMN default_value bigint NOT NULL DEFAULT 0;
			`,
		},
		{
			Version: "610",
			Up: `
				ALTER TABLE users ADD COLUMN service_account bool DEFAULT FALSE;
			`,
		},
		{
			Version: "620",
			Up: `
				ALTER TABLE plans ADD COLUMN most_popular bool DEFAULT FALSE;
			`,
		},
		{
			Version: "625",
			Up: `
				ALTER TABLE templates ADD COLUMN type character varying(255) DEFAULT '' NOT NULL;
			`,
		},
		{
			Version: "630",
			Up: `
				ALTER TABLE containers ALTER COLUMN slug TYPE character varying(63) USING substr(slug, 1, 63);
			`,
		},
		{
			Version: "640",
			Up: `
				ALTER TABLE tokens ADD COLUMN last_seen_at timestamp without time zone DEFAULT now();
				ALTER TABLE tokens ADD COLUMN last_ip character varying(255) DEFAULT NULL;
				ALTER TABLE tokens ADD COLUMN type character varying(255) DEFAULT 'login' NOT NULL;
			`,
		},
		{
			Version: "650",
			Up: `
				ALTER TABLE containers ADD column metadata hstore;
			`,
		},
		{
			Version: "660",
			Up: `
				ALTER TABLE user_profiles ADD COLUMN activation_email_sent_at timestamp without time zone DEFAULT NULL;
				ALTER TABLE user_profiles ADD COLUMN starter_credit_email_send_count int NOT NULL DEFAULT 0;
				ALTER TABLE user_profiles ADD COLUMN starter_credit_email_last_sent_at timestamp without time zone DEFAULT NULL;
			`,
		},
		{
			Version: "670",
			Up: `
				ALTER TABLE provider_templates DROP COLUMN hourly_price;
			`,
		},
		{
			Version: "680",
			Up: `
				ALTER TABLE users ADD COLUMN promo_code character varying(255) DEFAULT NULL;
				ALTER TABLE users DROP COLUMN invite_code_id;
				ALTER TABLE credit_types ADD COLUMN stock bigint DEFAULT NULL;
				ALTER TABLE credit_types ADD COLUMN used bigint DEFAULT NULL;
				ALTER TABLE credit_types ADD COLUMN expires_at timestamp without time zone;
				ALTER TABLE credit_types ADD CONSTRAINT overstock_check CHECK (stock IS NULL OR used <= stock);
			`,
		},
		{
			Version: "690",
			Up: `
				ALTER TABLE templates ADD COLUMN image character varying NOT NULL;
			`,
		},
		{
			Version: "700",
			Up: `
				ALTER TABLE regions ADD COLUMN disabled_at timestamp without time zone DEFAULT NULL;
			`,
		},
		{
			Version: "710",
			Up: `
				ALTER TABLE users DROP COLUMN billing_enabled;
			`,
		},
		{
			Version: "720",
			Up: `
				ALTER TABLE users ADD max_containers bigint DEFAULT NULL;
			`,
		},
		{
			Version: "730",
			Up: `
				ALTER TABLE containers ADD COLUMN temp_state bigint;
			`,
		},
		{
			Version: "740",
			Up: `
				ALTER TABLE invoice_items ADD COLUMN terminated bool DEFAULT FALSE;
			`,
		},
		{
			Version: "750",
			Up: `
				ALTER TABLE invoices ADD COLUMN available_credit bigint DEFAULT NULL;
			`,
		},
		{
			Version: "760",
			Up: `
				ALTER TABLE user_profiles ADD COLUMN github_user_name character varying(255) DEFAULT NULL;
				ALTER TABLE user_profiles ADD COLUMN github_oauth_token_json character varying(255) DEFAULT NULL;
			`,
		},
		{
			Version: "770",
			Up: `
				CREATE TABLE invoice_manual_attempts (
					id bigserial PRIMARY KEY NOT NULL,
					invoice_id bigint REFERENCES invoices(id) NOT NULL,
					success boolean DEFAULT FALSE,
					error_message character varying,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
			`,
		},
		{
			Version: "780",
			Up: `
				ALTER TABLE containers RENAME COLUMN locked_at TO locked_until;
			`,
		},
		{
			Version: "790",
			Up: `
				ALTER TABLE billing_profiles ALTER COLUMN state TYPE character varying(255);
				ALTER TABLE billing_profiles ALTER COLUMN state SET DEFAULT 'no_card';
				ALTER TABLE billing_profiles ALTER COLUMN state SET NOT NULL;
				DROP TYPE billing_state;
				UPDATE billing_profiles SET state='no_card' WHERE state='free';
			`,
		},
		{
			Version: "800",
			Up: `
				ALTER TABLE provider_templates ADD COLUMN supports_start_stop boolean DEFAULT false NOT NULL;
			`,
		},
		{
			Version: "810",
			Up: `
				CREATE TABLE user_settings (
					id bigserial PRIMARY KEY NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					auto_close_brackets bool DEFAULT true NOT NULL,
					highlight_line bool DEFAULT true NOT NULL,
					keybindings character varying(255) DEFAULT 'default',
					match_brackets bool DEFAULT true NOT NULL,
					ruler_column bigint DEFAULT 0 NOT NULL,
					show_line_numbers bool DEFAULT true NOT NULL,
					show_trailing_space bool DEFAULT false NOT NULL,
					soft_tabs bool DEFAULT true NOT NULL,
					tab_size bigint DEFAULT 2 NOT NULL,
					theme character varying(255),
					wrap_lines bool DEFAULT true NOT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);
				CREATE UNIQUE INDEX index_user_settings_on_user_id ON user_settings USING btree (user_id);
			`,
		},
		{
			Version: "811",
			Up: `
				INSERT INTO user_settings (user_id) (
					SELECT users.id FROM users WHERE NOT EXISTS (SELECT user_id FROM user_settings)
				);
			`,
		},
		{
			Version: "812",
			Up: `
				UPDATE user_settings SET theme = 'solarized dark' WHERE theme IS NULL;
				ALTER TABLE user_settings ALTER COLUMN theme SET DEFAULT 'solarized dark';
				ALTER TABLE user_settings ALTER COLUMN theme SET NOT NULL;
				ALTER TABLE user_settings ALTER COLUMN keybindings SET NOT NULL;
			`,
		},
		{
			Version: "813",
			Up: `
				ALTER TABLE hosts ADD COLUMN region_identifier character varying(255);
			`,
		},
		{
			Version: "820",
			Up: `
				CREATE TABLE announcements (
					id bigserial PRIMARY KEY NOT NULL,
					subject_template text NOT NULL,
					message_template text NOT NULL,
					show_to_all boolean NOT NULL,
					expires_at timestamp without time zone DEFAULT now(),
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);

				CREATE TABLE announcement_host (
					id bigserial PRIMARY KEY NOT NULL,
					announcement_id bigint REFERENCES announcements(id) NOT NULL,
					host_id bigint REFERENCES hosts(id) NOT NULL
				);

				CREATE UNIQUE INDEX index_announcement_id_host_id ON announcement_host USING btree (announcement_id, host_id);
				`,
		},
		{
			Version: "830",
			Up: `
				ALTER TABLE plans ADD COLUMN base_price bigint DEFAULT 0 NOT NULL;
			`,
		},
		{
			Version: "831",
			Up: `
				ALTER TABLE invoice_items ADD COLUMN item_type character varying(255);
				UPDATE invoice_items SET item_type = 'credit' WHERE credit > 0;
				UPDATE invoice_items SET item_type = 'compute' WHERE item_type IS NULL;
				ALTER TABLE invoice_items ALTER COLUMN item_type SET NOT NULL;
			`,
		},
		{
			Version: "840",
			Up: `
				ALTER TABLE invoice_items ADD COLUMN plan_storage character varying(255) DEFAULT NULL;
				ALTER TABLE invoice_items ADD COLUMN plan_memory character varying(255) DEFAULT NULL;
				UPDATE invoice_items SET plan_storage = p.storage FROM (
					SELECT hosts.id host_id, storage FROM plans INNER JOIN hosts ON hosts.plan_id = plans.id
				) p WHERE p.host_id = invoice_items.host_id AND invoice_items.item_type = 'base_ebs';
				UPDATE invoice_items SET plan_memory = p.memory FROM (
					SELECT hosts.id host_id, memory FROM plans INNER JOIN hosts ON hosts.plan_id = plans.id
				) p WHERE p.host_id = invoice_items.host_id AND invoice_items.item_type = 'compute';
			`,
		},
		{
			Version: "850",
			Up: `
				UPDATE billing_profiles SET state='exempt' WHERE state='free';
				ALTER TABLE billing_profiles ADD COLUMN max_hours bigint DEFAULT NULL;
			`,
		},
		{
			Version: "851",
			Up: `
				ALTER TABLE invoices ADD COLUMN state character varying(255) DEFAULT 'unpaid';
				UPDATE invoices SET state = 'failed' WHERE failed = true;
				UPDATE invoices SET state = 'deferred' WHERE deferred = true;
				UPDATE invoices SET state = 'paid' WHERE successful_charge = true;
				UPDATE invoices SET state = 'not_payable' WHERE total_charge = 0;
				ALTER TABLE invoices ALTER COLUMN state SET DEFAULT NULL;
				ALTER TABLE invoices ALTER COLUMN state SET NOT NULL;
			`,
		},
		{
			Version: "852",
			Up: `
				ALTER TABLE provider_templates ADD COLUMN disabled boolean NOT NULL default false;
			`,
		},
		{
			Version: "860",
			Up: `
				CREATE TABLE container_collabs (
					id bigserial PRIMARY KEY NOT NULL,
					container_id bigint REFERENCES containers(id) NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					email_sent_at timestamp without time zone DEFAULT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);

				CREATE UNIQUE INDEX index_container_collabs_user_id_container_id ON container_collabs USING btree (user_id, container_id);
			`,
		},
		{
			Version: "870",
			Up: `
				ALTER TABLE billing_profiles ADD COLUMN free_trial_expires_at timestamp without time zone;
			`,
		},
		{
			Version: "871",
			Up: `
				ALTER TABLE billing_profiles DROP COLUMN free_trial_expires_at;
			`,
		},
		{
			Version: "880",
			Up: `
				ALTER TABLE users ADD COLUMN first_time bool DEFAULT TRUE NOT NULL;
				UPDATE users SET first_time = FALSE;
			`,
		},
		{
			Version: "890",
			Up: `
				ALTER TABLE users ADD COLUMN phone character varying(255) DEFAULT NULL;
				ALTER TABLE users ADD COLUMN phone_country_code character varying(255) DEFAULT NULL;
				ALTER TABLE users ADD COLUMN phone_authed boolean DEFAULT false NOT NULL;
				CREATE UNIQUE INDEX index_users_on_phone_and_phone_country_code ON users USING btree (phone, phone_country_code);
			`,
		},
		{
			Version: "900",
			Up: `
				CREATE TABLE verifications (
					id bigserial PRIMARY KEY NOT NULL,
					user_id bigint REFERENCES users(id) NOT NULL,
					phone character varying(255) NOT NULL,
					code character varying(63) NOT NULL,
					created_at timestamp without time zone DEFAULT now(),
					updated_at timestamp without time zone DEFAULT now()
				);

				-- Allow only one verification at a time (i.e. only 1 phone per
			    -- user) since we don't need to support multiple phones, for now.
				CREATE UNIQUE INDEX index_verifications_on_user_id ON verifications USING btree (user_id);
			`,
		},
		{
			Version: "910",
			Up: `
				ALTER TABLE users ADD COLUMN max_running_time bigint DEFAULT 7200;
				ALTER TABLE users ADD COLUMN refresh_period bigint DEFAULT 86400;
				UPDATE users SET max_running_time = NULL, refresh_period = NULL;
			`,
		},
	}
}
