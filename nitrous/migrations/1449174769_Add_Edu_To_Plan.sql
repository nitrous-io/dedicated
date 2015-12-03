-- Add_Edu_To_Plan
-- Version: 1449174769

ALTER TABLE plans ADD COLUMN edu boolean DEFAULT false;
ALTER TABLE orgs ADD COLUMN edu boolean DEFAULT false;
