-- add_user_id_index_to_tokens
-- Version: 1454307276

CREATE INDEX index_tokens_on_user_id ON tokens USING btree (user_id);
