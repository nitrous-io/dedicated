-- Create_Edu_Emails_Whitelist
-- Version: 1449183758

CREATE TABLE edu_emails_whitelist (
  id bigserial PRIMARY KEY NOT NULL,
  email character varying(255) NOT NULL
);

CREATE UNIQUE INDEX index_edu_emails_whitelist_on_email ON edu_emails_whitelist USING btree (email);
