-- create_promo_codes_table
-- Version: 1448397624

CREATE TABLE promo_codes (
  id bigserial PRIMARY KEY NOT NULL,
  slug character varying(30) NOT NULL UNIQUE,
  disabled boolean NOT NULL DEFAULT FALSE,
  quantity bigint NOT NULL,
  trial_seconds bigint DEFAULT NULL,
  expires_at timestamp without time zone DEFAULT NULL,
  created_at timestamp without time zone DEFAULT now(),
  updated_at timestamp without time zone DEFAULT now()
);

CREATE TABLE promo_code_uses (
  id bigserial PRIMARY KEY NOT NULL,
  promo_code_id bigint REFERENCES promo_codes(id) NOT NULL,
  billing_profile_id bigint REFERENCES billing_profiles(id) NOT NULL,
  subscription_id bigint REFERENCES subscriptions(id) DEFAULT NULL,
  created_at timestamp without time zone DEFAULT now(),
  updated_at timestamp without time zone DEFAULT now()
);

COMMENT ON TABLE promo_codes IS 'Controls the promotions (trial period) when creating a new subscription';
COMMENT ON COLUMN promo_codes.trial_seconds IS 'Specify in seconds, NULL implies no trial';
