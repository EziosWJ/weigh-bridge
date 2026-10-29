-- +goose Up
ALTER TABLE wb_ticket
    ADD COLUMN net_adjustment_milli_tonne BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN net_adjustment_mode VARCHAR(10) NOT NULL DEFAULT 'amount',
    ADD COLUMN net_adjustment_rate_bp INTEGER,
    ADD COLUMN net_adjustment_category VARCHAR(40) NOT NULL DEFAULT '',
    ADD COLUMN net_adjustment_category_name VARCHAR(100) NOT NULL DEFAULT '';

ALTER TABLE wb_ticket
    ADD CONSTRAINT ck_wb_ticket_net_adjustment_mode
        CHECK (net_adjustment_mode IN ('amount','percent')),
    ADD CONSTRAINT ck_wb_ticket_net_adjustment_rate
        CHECK (
            (net_adjustment_mode='amount' AND net_adjustment_rate_bp IS NULL)
            OR (net_adjustment_mode='percent' AND net_adjustment_rate_bp IS NOT NULL AND net_adjustment_rate_bp BETWEEN -10000 AND 10000)
        ),
    ADD CONSTRAINT ck_wb_ticket_final_net
        CHECK (
            final_gross_milli_tonne - final_tare_milli_tonne - net_adjustment_milli_tonne > 0
            AND final_gross_milli_tonne - final_tare_milli_tonne - net_adjustment_milli_tonne < final_gross_milli_tonne
        );

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Net adjustment evidence cannot be discarded safely; restore the pre-upgrade database backup'; END $$;
-- +goose StatementEnd
