-- Rename_PromoCode_On_User
-- Version: 1449086225

ALTER TABLE users RENAME COLUMN promo_code TO previous_promo_code;
