-- add_unavailable_at_to_hosts
-- Version: 1447908398

ALTER TABLE hosts ADD COLUMN unavailable_at timestamp without time zone DEFAULT NULL;
