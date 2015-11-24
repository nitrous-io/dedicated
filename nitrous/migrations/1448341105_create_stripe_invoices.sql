-- create_stripe_invoices
-- Version: 1448341105

CREATE TABLE stripe_invoices (
	billing_profile_id bigint REFERENCES billing_profiles(id) NOT NULL,
	invoice_json JSON DEFAULT '{}'::JSON NOT NULL,
	created_at timestamp without time zone DEFAULT now() NOT NULL
);

CREATE INDEX index_stripe_invoices_on_bp_id ON stripe_invoices USING btree (billing_profile_id);
