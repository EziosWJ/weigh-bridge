-- +goose Up
ALTER TABLE wb_sample ALTER COLUMN expires_at DROP NOT NULL;

-- +goose Down
UPDATE wb_sample SET expires_at = captured_at + INTERVAL '30 seconds' WHERE expires_at IS NULL;
ALTER TABLE wb_sample ALTER COLUMN expires_at SET NOT NULL;
