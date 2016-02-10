-- create_bonuses
-- Version: 1453859065

CREATE TABLE bonuses (
  id bigserial PRIMARY KEY NOT NULL,
  slug character varying(255) NOT NULL UNIQUE,
  amount bigint NOT NULL, -- usage value in seconds, corresponds with usage_quota value type in users table
  created_at timestamp without time zone DEFAULT now(),
  updated_at timestamp without time zone DEFAULT now()
);

CREATE TABLE earned_bonuses (
  id bigserial PRIMARY KEY NOT NULL,
  bonus_id bigint REFERENCES bonuses(id) NOT NULL,
  user_id bigint REFERENCES users(id) NOT NULL,
  created_at timestamp without time zone DEFAULT now(),
  updated_at timestamp without time zone DEFAULT now()
);
CREATE UNIQUE INDEX index_earned_bonuses_on_user_id_and_bonus_id ON earned_bonuses USING btree (user_id, bonus_id);

COMMENT ON TABLE bonuses IS 'Controls all permitted bonus tasks';
COMMENT ON TABLE earned_bonuses IS 'Tracks earned bonuses to prevent abuse';
