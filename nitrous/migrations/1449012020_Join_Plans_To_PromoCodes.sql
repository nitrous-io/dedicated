-- Join_Plans_To_PromoCodes
-- Version: 1449012020

CREATE TABLE promo_code_plans (
  id bigserial PRIMARY KEY NOT NULL,
  promo_code_id bigint REFERENCES promo_codes(id) NOT NULL,
  plan_id bigint REFERENCES plans(id) NOT NULL
);

CREATE UNIQUE INDEX index_promo_code_plans_on_promo_code_plan ON promo_code_plans USING btree (promo_code_id, plan_id);
