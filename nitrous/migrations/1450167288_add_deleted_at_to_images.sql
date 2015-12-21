-- add_deleted_at_to_images
-- Version: 1450167288
ALTER TABLE images ADD COLUMN deleted_at timestamp without time zone DEFAULT NULL;

