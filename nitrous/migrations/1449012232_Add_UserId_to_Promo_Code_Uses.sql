-- Add_UserId_to_Promo_Code_Uses
-- Version: 1449012232

ALTER TABLE promo_code_uses DROP COLUMN billing_profile_id;
ALTER TABLE promo_code_uses ADD COLUMN user_id bigint REFERENCES users(id) NOT NULL;

CREATE UNIQUE INDEX index_promo_code_uses_on_user_promo ON promo_code_uses USING btree (user_id, promo_code_id);
