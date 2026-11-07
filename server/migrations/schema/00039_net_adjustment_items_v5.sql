-- +goose Up
-- 多项净重调整（见 docs/adr/0036-net-weight-adjustment-items.md）：
-- 明细整体作为单据快照列保存，历史单项按已有字段展示，不回填。

ALTER TABLE wb_ticket
    ADD COLUMN net_adjustment_items JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE wb_ticket
    ADD CONSTRAINT ck_wb_ticket_net_adjustment_items
        CHECK (jsonb_typeof(net_adjustment_items) = 'array');

-- v5 摘要协议待上传变化。旧版 v3/v4 变化保留原表、原协议、原载荷与摘要。
CREATE TABLE wb_ticket_sync_change_v5 (
    id BIGSERIAL PRIMARY KEY,
    ticket_id BIGINT NOT NULL,
    previous_ticket_id BIGINT,
    digest VARCHAR(64) NOT NULL,
    digest_version INTEGER NOT NULL,
    schema_version INTEGER NOT NULL,
    site_code VARCHAR(20) NOT NULL,
    ticket_no VARCHAR(200) NOT NULL,
    change_type VARCHAR(10) NOT NULL CHECK (change_type IN ('issue','void','reopen')),
    payload JSONB NOT NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','waiting_predecessor','temporary_failure','needs_admin','conflict','synced')),
    category VARCHAR(30) NOT NULL DEFAULT '',
    reason_code VARCHAR(60) NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    last_attempt_at TIMESTAMPTZ,
    synced_at TIMESTAMPTZ,
    received_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_wb_ticket_sync_change_v5_ticket ON wb_ticket_sync_change_v5 (ticket_id, id);
CREATE INDEX idx_wb_ticket_sync_change_v5_due ON wb_ticket_sync_change_v5 (status, next_attempt_at);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Net adjustment item evidence cannot be discarded safely; restore the pre-upgrade database backup'; END $$;
-- +goose StatementEnd
