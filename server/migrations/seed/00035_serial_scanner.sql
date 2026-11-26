-- +goose Up
-- Singleton rows make the configuration and status contracts explicit before
-- the first Agent heartbeat arrives.
INSERT INTO wb_serial_scanner (id) VALUES (1)
ON CONFLICT (id) DO NOTHING;

INSERT INTO wb_serial_scanner_status (id) VALUES (1)
ON CONFLICT (id) DO NOTHING;

-- +goose Down
DELETE FROM wb_serial_scanner_status WHERE id = 1;
DELETE FROM wb_serial_scanner WHERE id = 1;
