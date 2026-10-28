-- +goose Up
INSERT INTO wb_master_sync_state (id) VALUES (1) ON CONFLICT (id) DO NOTHING;
-- +goose Down
DELETE FROM wb_master_sync_state WHERE id = 1 AND bound = FALSE;
