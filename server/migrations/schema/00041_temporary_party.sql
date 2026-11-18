-- +goose Up
ALTER TABLE wb_party
    ADD COLUMN temporary_state VARCHAR(20) NOT NULL DEFAULT '',
    ADD COLUMN temporary_site_code VARCHAR(40) NOT NULL DEFAULT '',
    ADD COLUMN merged_party_code VARCHAR(40) NOT NULL DEFAULT '',
    ADD CONSTRAINT wb_party_temporary_state_check CHECK (
        (temporary_state = '' AND temporary_site_code = '' AND merged_party_code = '')
        OR (temporary_state IN ('pending', 'needs_info') AND temporary_site_code <> '' AND merged_party_code = '' AND enabled = 1)
        OR (temporary_state = 'disabled' AND temporary_site_code <> '' AND merged_party_code = '' AND enabled = 0)
        OR (temporary_state = 'merged' AND temporary_site_code <> '' AND merged_party_code <> '' AND enabled = 0)
    );

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM wb_party WHERE temporary_state <> '') THEN
        RAISE EXCEPTION 'Temporary party records contain durable registrations and merge results';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE wb_party
    DROP CONSTRAINT wb_party_temporary_state_check,
    DROP COLUMN temporary_state,
    DROP COLUMN temporary_site_code,
    DROP COLUMN merged_party_code;
