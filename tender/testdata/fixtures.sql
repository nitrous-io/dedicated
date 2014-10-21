--
-- Prepare tables structure
--

-- hosts
DROP TABLE IF EXISTS hosts;
CREATE TABLE hosts (
  id bigserial PRIMARY KEY NOT NULL,
  slug character varying(63) NOT NULL, -- Used as DNS label, see http://tools.ietf.org/html/rfc1034#section-3.1.
  ip_address character varying(45) -- See http://tools.ietf.org/html/rfc4291.
);

--
-- Prepare data
--

-- hosts
INSERT INTO hosts (slug, ip_address) VALUES ('oldapp', '10.0.0.1');
INSERT INTO hosts (slug, ip_address) VALUES ('newapp', '10.0.0.2');
