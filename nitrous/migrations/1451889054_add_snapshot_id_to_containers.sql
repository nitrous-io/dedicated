-- add_snapshot_id_to_containers
-- Version: 1451889054

ALTER TABLE containers ADD COLUMN snapshot_id bigint REFERENCES snapshots(id) DEFAULT NULL;
