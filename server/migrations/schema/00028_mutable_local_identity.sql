-- +goose Up
-- Current identity codes are editable configuration values. Historical
-- business, evidence, authorization, session and Agent rows no longer depend
-- on these foreign keys; the old user-site table is retained only for upgrade
-- compatibility and is no longer read or written by the application.
ALTER TABLE wb_scale DROP CONSTRAINT fk_wb_scale_site;
ALTER TABLE wb_local_identity DROP CONSTRAINT fk_wb_identity_site;
ALTER TABLE wb_user_site DROP CONSTRAINT fk_wb_user_site_site;

-- +goose Down
-- Snapshot codes can diverge from the current values after the first edit.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Mutable local identity migration is not automatically reversible; restore the pre-upgrade database backup'; END $$;
-- +goose StatementEnd
