-- create_bonus
-- Version: 1453859065

CREATE TABLE bonus (
  id bigserial PRIMARY KEY NOT NULL,
  slug character varying(255) NOT NULL,
  amount bigint NOT NULL, -- usage value in seconds, corresponds with usage_quota value type in users table
  created_at timestamp without time zone DEFAULT now(),
  updated_at timestamp without time zone DEFAULT now()
);

CREATE TABLE bonus_uses (
  id bigserial PRIMARY KEY NOT NULL,
  bonus_id bigint REFERENCES bonus(id) NOT NULL,
  user_id bigint REFERENCES users(id) NOT NULL,
  created_at timestamp without time zone DEFAULT now(),
  updated_at timestamp without time zone DEFAULT now()
);
CREATE UNIQUE INDEX index_bonus_uses_on_user_id_and_bonus_id ON bonus_uses USING btree (user_id, bonus_id);

COMMENT ON TABLE bonus IS 'Controls all permitted bonus tasks';
COMMENT ON TABLE bonus_uses IS 'Tracks used bonuses to prevent abuse';
