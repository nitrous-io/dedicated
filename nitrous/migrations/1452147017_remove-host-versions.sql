-- remove-host-versions
-- Version: 1452147017

ALTER TABLE hosts
  DROP COLUMN freighterd_version,
  DROP COLUMN docker_version;
