--
-- Prepare tables structure
--

-- containers
DROP TABLE IF EXISTS containers;
DROP TABLE IF EXISTS hosts;

-- hosts
CREATE TABLE hosts (
  id bigserial PRIMARY KEY NOT NULL,
  slug character varying(63) NOT NULL, -- Used as DNS label, see http://tools.ietf.org/html/rfc1034#section-3.1.
  ip_address character varying(45), -- See http://tools.ietf.org/html/rfc4291.
  secret character varying(255) NOT NULL DEFAULT left(encode(gen_random_bytes(191), 'base64'), 255),
  freighterd_port bigint,
  terminated_at timestamp without time zone,
  state character varying(255)
);

CREATE TABLE containers (
  id bigserial PRIMARY KEY NOT NULL,
  cid character varying(255) NOT NULL,
  slug character varying(63) NOT NULL, -- Used as DNS label, see http://tools.ietf.org/html/rfc1034#section-3.1.
  host_id bigint REFERENCES hosts(id) NOT NULL,
  terminated_at timestamp without time zone
);

--
-- Prepare data
--

-- hosts
INSERT INTO hosts (slug, ip_address, freighterd_port, state)
     VALUES ('oldapp', '10.0.0.1', 10001, 'running');
INSERT INTO hosts (slug, ip_address, freighterd_port, state)
     VALUES ('newapp', '10.0.0.2', 10002, 'running');
 INSERT INTO hosts (slug, ip_address, freighterd_port, state)
      VALUES ('local', '127.0.0.1', 66000, 'running');

-- containers
INSERT INTO containers (slug, cid, host_id)
     VALUES ('rails-app', 'asfd', (SELECT id FROM hosts WHERE slug='oldapp' LIMIT 1));
INSERT INTO containers (slug, cid, host_id)
     VALUES ('python-app', 'qwer', (SELECT id FROM hosts WHERE slug='oldapp' LIMIT 1));
INSERT INTO containers (slug, cid, host_id)
     VALUES ('nodejs-app', 'zxcv', (SELECT id FROM hosts WHERE slug='newapp' LIMIT 1));
INSERT INTO containers (slug, cid, host_id)
     VALUES ('local-app', 'zxcv', (SELECT id FROM hosts WHERE slug='local' LIMIT 1));
