-- add_invite_email_support_to_org_memberships
-- Version: 1450668799

ALTER TABLE org_memberships
  ALTER COLUMN user_id DROP NOT NULL,
  ADD COLUMN email_sent_at timestamp without time zone DEFAULT NULL,
  ADD COLUMN invite_email character varying(255) CHECK (user_id IS NOT NULL OR invite_email IS NOT NULL) DEFAULT NULL,
  ADD COLUMN invite_code character varying(20) DEFAULT NULL;

CREATE UNIQUE INDEX index_org_memberships_on_org_id_invite_email ON org_memberships USING btree (org_id, invite_email);
