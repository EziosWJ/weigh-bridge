-- +goose Up
CREATE TABLE wb_site (
    id BIGINT PRIMARY KEY,
    code VARCHAR(40) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    enabled SMALLINT NOT NULL DEFAULT 1 CHECK (enabled IN (0,1))
);
ALTER TABLE wb_scale ADD COLUMN site_id BIGINT NOT NULL DEFAULT 1 CHECK (site_id>0);
ALTER TABLE wb_transport_task ADD COLUMN site_id BIGINT NOT NULL DEFAULT 1 CHECK (site_id>0);
ALTER TABLE wb_ticket ADD COLUMN site_id BIGINT NOT NULL DEFAULT 1 CHECK (site_id>0);
ALTER TABLE wb_ticket ADD COLUMN site_name VARCHAR(100) NOT NULL DEFAULT '';
CREATE INDEX idx_wb_scale_site ON wb_scale(site_id);
CREATE INDEX idx_wb_task_site ON wb_transport_task(site_id,create_time,id);
CREATE INDEX idx_wb_ticket_site ON wb_ticket(site_id,issued_at,id);
DROP INDEX uk_wb_task_active_plate;
CREATE UNIQUE INDEX uk_wb_task_active_plate ON wb_transport_task(site_id,plate) WHERE status='awaiting_second';
CREATE TABLE wb_user_site (
    user_id BIGINT NOT NULL REFERENCES sys_user(id) ON DELETE CASCADE,
    site_id BIGINT NOT NULL REFERENCES wb_site(id) ON DELETE RESTRICT,
    can_operate SMALLINT NOT NULL DEFAULT 0 CHECK (can_operate IN (0,1)),
    PRIMARY KEY (user_id,site_id)
);

-- +goose Down
-- Roll back only after resolving active same-plate tasks across sites.
DROP INDEX uk_wb_task_active_plate;
CREATE UNIQUE INDEX uk_wb_task_active_plate ON wb_transport_task(plate) WHERE status='awaiting_second';
DROP TABLE wb_user_site;
DROP INDEX idx_wb_ticket_site;
DROP INDEX idx_wb_task_site;
DROP INDEX idx_wb_scale_site;
ALTER TABLE wb_ticket DROP COLUMN site_name;
ALTER TABLE wb_ticket DROP COLUMN site_id;
ALTER TABLE wb_transport_task DROP COLUMN site_id;
ALTER TABLE wb_scale DROP COLUMN site_id;
DROP TABLE wb_site;
