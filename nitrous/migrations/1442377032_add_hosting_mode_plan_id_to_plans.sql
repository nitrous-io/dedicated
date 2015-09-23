-- add_hosting_mode_plan_id_to_plans
-- Version: 1442377032

ALTER TABLE plans ADD COLUMN hosting_mode_plan_id bigint REFERENCES plans(id) DEFAULT NULL;
