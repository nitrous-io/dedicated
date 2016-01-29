-- remove_usage_remaining_column
-- Version: 1454044731

ALTER TABLE users
  DROP COLUMN usage_remaining;
