-- add_original_image_and_is_snapshot_to_images
-- Version: 1450341108
ALTER TABLE images ADD COLUMN original_image character varying(255) NULL; -- Docker image that container was created from
ALTER TABLE images ADD COLUMN is_snapshot bool DEFAULT false NOT NULL;

UPDATE images SET is_snapshot = true WHERE tag LIKE '__snapshot__%';
