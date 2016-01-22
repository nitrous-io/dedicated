-- add_metadata_to_host_transitions
-- Version: 1453438580

ALTER TABLE host_transitions ADD COLUMN metadata HSTORE;
