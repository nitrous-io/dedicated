-- add_source_ami_to_hosts
-- Version: 1448259457

ALTER TABLE hosts ADD COLUMN source_ami varchar(12) DEFAULT NULL;
