-- drop_scan_exempted_and_next_scan_from_hosts
-- Version: 1455780518

ALTER TABLE hosts DROP COLUMN next_scan;
ALTER TABLE hosts DROP COLUMN scan_exempted;
