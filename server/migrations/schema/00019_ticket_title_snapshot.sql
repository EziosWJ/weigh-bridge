-- +goose Up
-- ADR 0030 / DC-06: freeze the effective ticket title on each generated
-- ticket. Existing tickets keep an empty marker until the API start freezes the
-- upgrade-time baseline, so later configuration edits never rewrite history.
ALTER TABLE wb_ticket ADD COLUMN ticket_title VARCHAR(100) NOT NULL DEFAULT '';

-- Server-acquired camera evidence is a distinct sample source. Historical
-- simulated and Agent-uploaded rows keep their original values.
ALTER TABLE wb_sample DROP CONSTRAINT wb_sample_source_check;
ALTER TABLE wb_sample ADD CONSTRAINT wb_sample_source_check CHECK (source IN ('simulated','agent','server'));

-- +goose Down
-- Local-only cutover rollback is intentionally not in-place: later migrations
-- refuse it. Keep the widened source check when Server evidence exists so a
-- development rollback does not rewrite history or fail on committed rows.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM wb_sample WHERE source='server') THEN
        ALTER TABLE wb_sample DROP CONSTRAINT wb_sample_source_check;
        ALTER TABLE wb_sample ADD CONSTRAINT wb_sample_source_check CHECK (source IN ('simulated','agent'));
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE wb_ticket DROP COLUMN ticket_title;
