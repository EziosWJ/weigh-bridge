-- +goose Up
-- Issued ticket and captured evidence codes record the identity at the time of
-- the event. They are snapshots, not links to the currently configured code.
ALTER TABLE wb_ticket DROP CONSTRAINT fk_wb_ticket_site;
ALTER TABLE wb_sample DROP CONSTRAINT fk_wb_sample_scale;
ALTER TABLE wb_weigh_record DROP CONSTRAINT fk_wb_record_scale;
ALTER TABLE wb_weighing_image DROP CONSTRAINT fk_wb_image_scale;

-- +goose Down
-- Snapshot codes may no longer match the current configured identity.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Local evidence snapshot migration is not automatically reversible; restore the pre-upgrade database backup'; END $$;
-- +goose StatementEnd
