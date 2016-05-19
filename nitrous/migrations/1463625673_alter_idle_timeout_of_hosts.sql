-- alter_idle_timeout_of_hosts
-- Version: 1463625673

 ALTER TABLE ONLY hosts ALTER COLUMN idle_timeout SET DEFAULT 2400;
