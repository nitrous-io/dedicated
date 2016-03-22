-- create_referral_codes
-- Version: 1458074050

CREATE TABLE referral_codes (
  id bigserial PRIMARY KEY NOT NULL,
  user_id bigint REFERENCES users(id) NOT NULL UNIQUE,

  code character varying(255) NOT NULL UNIQUE,
  bonus bigint NOT NULL DEFAULT 10800, -- bonus usage in seconds, corresponds with usage_quota value type in users table
  cap bigint NOT NULL DEFAULT 10,

  created_at timestamp without time zone DEFAULT now(),
  updated_at timestamp without time zone DEFAULT now()
);

CREATE TABLE referral_code_uses (
  id bigserial PRIMARY KEY NOT NULL,
  referral_code_id bigint REFERENCES referral_codes(id) NOT NULL,
  user_id bigint REFERENCES users(id) NOT NULL UNIQUE,
  created_at timestamp without time zone DEFAULT now(),
  updated_at timestamp without time zone DEFAULT now()
);

COMMENT ON TABLE referral_codes IS 'Stores the bonus hours available to user for each referral';
