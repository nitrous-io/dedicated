--
-- Prepare tables structure
--

-- users
DROP TABLE IF EXISTS users;
CREATE TABLE users (
  id bigserial PRIMARY KEY NOT NULL,
  username character varying(30) NOT NULL,
  email character varying(255) NOT NULL
);

-- tokens
DROP TABLE IF EXISTS tokens;
CREATE TABLE tokens (
  id bigserial PRIMARY KEY NOT NULL,
  user_id bigint NOT NULL,
  token character varying(255) NOT NULL
);

-- hosts
DROP TABLE IF EXISTS hosts;
CREATE TABLE hosts (
  id bigserial PRIMARY KEY NOT NULL,
  user_id bigint NOT NULL,
  name character varying(63) NOT NULL, -- Used as DNS label, see http://tools.ietf.org/html/rfc1034#section-3.1.
  state character varying(255) NOT NULL DEFAULT 'born',
  ip_address character varying(45) -- See http://tools.ietf.org/html/rfc4291.
);

--
-- Prepare data
--

-- users
INSERT INTO users (username, email) VALUES ('admin', 'admin@nitrous.io');

-- tokens
INSERT INTO tokens (user_id, token) SELECT id, 'token' FROM users WHERE username='admin';
INSERT INTO tokens (user_id, token) VALUES (666, 'missing_user');

-- hosts
INSERT INTO hosts (user_id, name, state, ip_address) SELECT id, 'nitrous', 'running', '10.0.0.1' FROM users WHERE username='admin';
INSERT INTO hosts (user_id, name, state, ip_address) VALUES (666, 'missing_user', 'running', '10.0.0.2');
