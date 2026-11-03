-- +goose Up
-- A manual return weighing is a recorded business exception: it has no device
-- sample and must never be presented as a collected measurement. Historical
-- records keep 'collected'; only the explicit manual-return command writes
-- 'manual'.
ALTER TABLE wb_weigh_record ALTER COLUMN sample_id DROP NOT NULL;
ALTER TABLE wb_weigh_record ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'collected';
ALTER TABLE wb_weigh_record ADD CONSTRAINT ck_wb_weigh_record_source CHECK (source IN ('collected','manual'));

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Manual return evidence cannot be discarded safely; restore the pre-upgrade database backup'; END $$;
-- +goose StatementEnd
