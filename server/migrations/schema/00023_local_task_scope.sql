-- +goose Up
-- Task codes describe the identity when the task was created; they are not
-- local ownership keys. Single-site active plate uniqueness spans all codes.
ALTER TABLE wb_transport_task DROP CONSTRAINT fk_wb_task_site;
ALTER TABLE wb_transport_task DROP CONSTRAINT fk_wb_task_scale;
DROP INDEX uk_wb_task_active_plate;
CREATE UNIQUE INDEX uk_wb_task_active_plate ON wb_transport_task(plate) WHERE status='awaiting_second';

-- +goose Down
-- Historical task codes may diverge from the current identity after cutover.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Local task scope migration is not automatically reversible; restore the pre-upgrade database backup'; END $$;
-- +goose StatementEnd
