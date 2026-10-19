-- +goose Up
ALTER TABLE wb_transport_task DROP CONSTRAINT wb_transport_task_mode_check;
ALTER TABLE wb_transport_task ADD CONSTRAINT wb_transport_task_mode_check CHECK (mode IN ('test','normal'));
ALTER TABLE wb_ticket DROP CONSTRAINT wb_ticket_mode_check;
ALTER TABLE wb_ticket ADD CONSTRAINT wb_ticket_mode_check CHECK (mode IN ('test','normal'));
ALTER TABLE wb_sample DROP CONSTRAINT wb_sample_source_check;
ALTER TABLE wb_sample ADD CONSTRAINT wb_sample_source_check CHECK (source IN ('simulated','agent'));
ALTER TABLE wb_print_attempt DROP CONSTRAINT wb_print_attempt_resolution_source_check;
ALTER TABLE wb_print_attempt ADD CONSTRAINT wb_print_attempt_resolution_source_check CHECK (resolution_source IN ('operator','agent'));
ALTER TABLE wb_number_sequence DROP CONSTRAINT wb_number_sequence_kind_check;
ALTER TABLE wb_number_sequence ADD CONSTRAINT wb_number_sequence_kind_check CHECK (kind IN ('task','test_ticket','ticket','material','party'));

-- +goose Down
-- Deliberately keep widened value constraints: tightening would destroy or reject
-- Agent business history. The preceding workflow migration still drops its tables.
SELECT 1;
