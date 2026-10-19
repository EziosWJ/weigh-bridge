-- +goose NO TRANSACTION
-- +goose Up
-- SQLite requires a table rebuild to widen CHECK constraints. Keep every row,
-- foreign-key reference and index; disable FK enforcement outside the transaction.
PRAGMA foreign_keys=OFF;
BEGIN IMMEDIATE;
CREATE TABLE wb_scale_session_agent_new (
    scale_id INTEGER PRIMARY KEY REFERENCES wb_scale(id) ON DELETE RESTRICT,
    holder_user_id INTEGER,
    holder_name VARCHAR(100),
    client_id VARCHAR(100),
    token_hash VARCHAR(64),
    generation INTEGER NOT NULL DEFAULT 0,
    expires_at DATETIME,
    version INTEGER NOT NULL DEFAULT 1,
    conditions_version INTEGER NOT NULL DEFAULT 1,
    test_conditions_snapshot TEXT NOT NULL DEFAULT '{}'
);
INSERT INTO wb_scale_session_agent_new SELECT * FROM wb_scale_session;
CREATE TABLE wb_transport_task_agent_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_no VARCHAR(40) NOT NULL UNIQUE,
    mode VARCHAR(10) NOT NULL DEFAULT 'test' CHECK (mode IN ('test','normal')),
    plate VARCHAR(20) NOT NULL,
    direction VARCHAR(10) NOT NULL CHECK (direction IN ('inbound','outbound')),
    status VARCHAR(30) NOT NULL CHECK (status IN ('awaiting_second','completed','terminated')),
    scale_id INTEGER NOT NULL REFERENCES wb_scale(id) ON DELETE RESTRICT,
    material_id INTEGER NOT NULL REFERENCES wb_material(id) ON DELETE RESTRICT,
    party_id INTEGER NOT NULL REFERENCES wb_party(id) ON DELETE RESTRICT,
    carrier_id INTEGER REFERENCES wb_party(id) ON DELETE RESTRICT,
    business_snapshot TEXT NOT NULL,
    current_first_record_id INTEGER,
    current_second_record_id INTEGER,
    version INTEGER NOT NULL DEFAULT 1 CHECK (version>0),
    create_time DATETIME NOT NULL,
    update_time DATETIME NOT NULL,
    actor_id INTEGER NOT NULL,
    actor_name VARCHAR(100) NOT NULL,
    completed_at DATETIME,
    terminated_at DATETIME,
    terminate_reason TEXT,
    FOREIGN KEY (id,current_first_record_id) REFERENCES wb_weigh_record(task_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (id,current_second_record_id) REFERENCES wb_weigh_record(task_id,id) ON DELETE RESTRICT
);
INSERT INTO wb_transport_task_agent_new SELECT * FROM wb_transport_task;
CREATE TABLE wb_sample_agent_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scale_id INTEGER NOT NULL REFERENCES wb_scale(id) ON DELETE RESTRICT,
    owner_user_id INTEGER NOT NULL,
    client_id VARCHAR(100) NOT NULL,
    session_generation INTEGER NOT NULL,
    task_id INTEGER REFERENCES wb_transport_task(id) ON DELETE RESTRICT,
    target_record_id INTEGER,
    task_version INTEGER,
    business_hash VARCHAR(64) NOT NULL,
    business_snapshot TEXT NOT NULL,
    stage VARCHAR(10) NOT NULL CHECK (stage IN ('first','second')),
    kind VARCHAR(10) NOT NULL CHECK (kind IN ('gross','tare')),
    plate VARCHAR(20) NOT NULL,
    source VARCHAR(20) NOT NULL CHECK (source IN ('simulated','agent')),
    weight_milli_tonne INTEGER NOT NULL CHECK (weight_milli_tonne>0),
    captured_at DATETIME NOT NULL,
    expires_at DATETIME NOT NULL,
    stable INTEGER NOT NULL CHECK (stable IN (0,1)),
    device_ok INTEGER NOT NULL CHECK (device_ok IN (0,1)),
    evidence_snapshot TEXT NOT NULL,
    plate_evidence TEXT NOT NULL,
    conditions_version INTEGER NOT NULL,
    invalidated_at DATETIME,
    FOREIGN KEY (task_id,target_record_id) REFERENCES wb_weigh_record(task_id,id) ON DELETE RESTRICT
);
INSERT INTO wb_sample_agent_new SELECT * FROM wb_sample;
CREATE TABLE wb_weigh_record_agent_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id INTEGER NOT NULL REFERENCES wb_transport_task(id) ON DELETE RESTRICT,
    sample_id INTEGER NOT NULL UNIQUE REFERENCES wb_sample(id) ON DELETE RESTRICT,
    stage VARCHAR(10) NOT NULL CHECK (stage IN ('first','second')),
    kind VARCHAR(10) NOT NULL CHECK (kind IN ('gross','tare')),
    weight_milli_tonne INTEGER NOT NULL CHECK (weight_milli_tonne>0),
    captured_at DATETIME NOT NULL,
    confirmed_at DATETIME NOT NULL,
    scale_id INTEGER NOT NULL REFERENCES wb_scale(id) ON DELETE RESTRICT,
    replaces_record_id INTEGER UNIQUE REFERENCES wb_weigh_record(id) ON DELETE RESTRICT,
    retained_record_id INTEGER REFERENCES wb_weigh_record(id) ON DELETE RESTRICT,
    reason TEXT,
    same_load_confirmed INTEGER NOT NULL DEFAULT 0 CHECK (same_load_confirmed IN (0,1)),
    other_basis_confirmed INTEGER NOT NULL DEFAULT 0 CHECK (other_basis_confirmed IN (0,1)),
    actor_id INTEGER NOT NULL,
    actor_name VARCHAR(100) NOT NULL,
    evidence_snapshot TEXT NOT NULL,
    plate_evidence TEXT NOT NULL,
    UNIQUE (task_id,id)
);
INSERT INTO wb_weigh_record_agent_new SELECT * FROM wb_weigh_record;
CREATE TABLE wb_ticket_agent_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket_no VARCHAR(40) NOT NULL UNIQUE,
    task_id INTEGER NOT NULL REFERENCES wb_transport_task(id) ON DELETE RESTRICT,
    revision INTEGER NOT NULL CHECK (revision>0),
    previous_ticket_id INTEGER UNIQUE,
    mode VARCHAR(10) NOT NULL DEFAULT 'test' CHECK (mode IN ('test','normal')),
    status VARCHAR(10) NOT NULL CHECK (status IN ('valid','void')),
    first_record_id INTEGER NOT NULL,
    second_record_id INTEGER NOT NULL,
    business_snapshot TEXT NOT NULL,
    basis_snapshot TEXT NOT NULL,
    snapshot_schema_version INTEGER NOT NULL DEFAULT 1,
    original_gross_milli_tonne INTEGER NOT NULL CHECK (original_gross_milli_tonne>0),
    original_tare_milli_tonne INTEGER NOT NULL CHECK (original_tare_milli_tonne>0),
    final_gross_milli_tonne INTEGER NOT NULL CHECK (final_gross_milli_tonne>0),
    final_tare_milli_tonne INTEGER NOT NULL CHECK (final_tare_milli_tonne>0),
    gross_source VARCHAR(20) NOT NULL CHECK (gross_source IN ('collected','manual')),
    tare_source VARCHAR(20) NOT NULL CHECK (tare_source IN ('collected','manual')),
    adjustment_remark TEXT NOT NULL DEFAULT '',
    adjusted_by INTEGER,
    adjusted_at DATETIME,
    issued_at DATETIME NOT NULL,
    issued_by INTEGER NOT NULL,
    issuer_name VARCHAR(100) NOT NULL,
    voided_at DATETIME,
    voided_by INTEGER,
    void_reason TEXT,
    UNIQUE (task_id,revision),
    UNIQUE (task_id,id),
    FOREIGN KEY (task_id,first_record_id) REFERENCES wb_weigh_record(task_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (task_id,second_record_id) REFERENCES wb_weigh_record(task_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (task_id,previous_ticket_id) REFERENCES wb_ticket(task_id,id) ON DELETE RESTRICT,
    CHECK (original_gross_milli_tonne>original_tare_milli_tonne),
    CHECK (final_gross_milli_tonne>final_tare_milli_tonne)
);
INSERT INTO wb_ticket_agent_new SELECT * FROM wb_ticket;
CREATE TABLE wb_operation_agent_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_id INTEGER NOT NULL,
    request_id VARCHAR(128) NOT NULL,
    command VARCHAR(60) NOT NULL,
    request_hash VARCHAR(64) NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('committed','rejected')),
    task_id INTEGER REFERENCES wb_transport_task(id) ON DELETE RESTRICT,
    ticket_id INTEGER REFERENCES wb_ticket(id) ON DELETE RESTRICT,
    result_snapshot TEXT NOT NULL,
    error_snapshot TEXT NOT NULL DEFAULT '',
    committed_at DATETIME NOT NULL,
    UNIQUE (actor_id,request_id)
);
INSERT INTO wb_operation_agent_new SELECT * FROM wb_operation;
CREATE TABLE wb_business_event_agent_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    operation_id INTEGER REFERENCES wb_operation(id) ON DELETE RESTRICT,
    task_id INTEGER REFERENCES wb_transport_task(id) ON DELETE RESTRICT,
    ticket_id INTEGER REFERENCES wb_ticket(id) ON DELETE RESTRICT,
    sample_id INTEGER REFERENCES wb_sample(id) ON DELETE RESTRICT,
    resource_type VARCHAR(60) NOT NULL,
    resource_id INTEGER,
    action VARCHAR(80) NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    before_snapshot TEXT,
    after_snapshot TEXT,
    actor_id INTEGER NOT NULL,
    actor_name VARCHAR(100) NOT NULL,
    occurred_at DATETIME NOT NULL
);
INSERT INTO wb_business_event_agent_new SELECT * FROM wb_business_event;
CREATE TABLE wb_print_attempt_agent_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket_id INTEGER NOT NULL REFERENCES wb_ticket(id) ON DELETE RESTRICT,
    request_id VARCHAR(128) NOT NULL,
    attempt_no INTEGER NOT NULL CHECK (attempt_no>0),
    intent VARCHAR(20) NOT NULL CHECK (intent IN ('initial','retry','reprint')),
    status VARCHAR(20) NOT NULL CHECK (status IN ('initiated','unknown','printed','failed')),
    reason TEXT NOT NULL DEFAULT '',
    initiated_at DATETIME NOT NULL,
    initiated_by INTEGER NOT NULL,
    resolved_at DATETIME,
    resolved_by INTEGER,
    resolution_source VARCHAR(20) NOT NULL DEFAULT 'operator' CHECK (resolution_source IN ('operator','agent')),
    closed_at DATETIME,
    close_reason TEXT NOT NULL DEFAULT '',
    UNIQUE (ticket_id,attempt_no)
);
INSERT INTO wb_print_attempt_agent_new SELECT * FROM wb_print_attempt;
CREATE TABLE wb_number_sequence_agent_new (
    kind VARCHAR(30) NOT NULL CHECK (kind IN ('task', 'test_ticket', 'ticket', 'material', 'party')),
    business_date VARCHAR(8) NOT NULL,
    next_value INTEGER NOT NULL CHECK (next_value > 0),
    PRIMARY KEY (kind, business_date)
);
INSERT INTO wb_number_sequence_agent_new SELECT * FROM wb_number_sequence;
DROP TABLE wb_number_sequence;
DROP TABLE wb_print_attempt;
DROP TABLE wb_business_event;
DROP TABLE wb_operation;
DROP TABLE wb_ticket;
DROP TABLE wb_weigh_record;
DROP TABLE wb_sample;
DROP TABLE wb_transport_task;
DROP TABLE wb_scale_session;
ALTER TABLE wb_scale_session_agent_new RENAME TO wb_scale_session;
ALTER TABLE wb_transport_task_agent_new RENAME TO wb_transport_task;
ALTER TABLE wb_sample_agent_new RENAME TO wb_sample;
ALTER TABLE wb_weigh_record_agent_new RENAME TO wb_weigh_record;
ALTER TABLE wb_ticket_agent_new RENAME TO wb_ticket;
ALTER TABLE wb_operation_agent_new RENAME TO wb_operation;
ALTER TABLE wb_business_event_agent_new RENAME TO wb_business_event;
ALTER TABLE wb_print_attempt_agent_new RENAME TO wb_print_attempt;
ALTER TABLE wb_number_sequence_agent_new RENAME TO wb_number_sequence;
CREATE UNIQUE INDEX uk_wb_task_active_plate ON wb_transport_task(plate) WHERE status='awaiting_second';
CREATE INDEX idx_wb_task_status_created ON wb_transport_task(status,create_time,id);
CREATE INDEX idx_wb_task_plate_created ON wb_transport_task(plate,create_time,id);
CREATE INDEX idx_wb_task_material ON wb_transport_task(material_id);
CREATE INDEX idx_wb_task_party ON wb_transport_task(party_id);
CREATE INDEX idx_wb_task_carrier ON wb_transport_task(carrier_id);
CREATE INDEX idx_wb_sample_scale_time ON wb_sample(scale_id,captured_at,id);
CREATE INDEX idx_wb_record_task_confirmed ON wb_weigh_record(task_id,confirmed_at,id);
CREATE INDEX idx_wb_record_stage_confirmed ON wb_weigh_record(stage,confirmed_at,id);
CREATE UNIQUE INDEX uk_wb_ticket_valid ON wb_ticket(task_id) WHERE status='valid';
CREATE INDEX idx_wb_ticket_issued ON wb_ticket(issued_at,id);
CREATE INDEX idx_wb_ticket_task_issued ON wb_ticket(task_id,issued_at,id);
CREATE INDEX idx_wb_event_task_time ON wb_business_event(task_id,occurred_at,id);
CREATE INDEX idx_wb_event_ticket_time ON wb_business_event(ticket_id,occurred_at,id);
CREATE INDEX idx_wb_event_resource ON wb_business_event(resource_type,resource_id);
CREATE UNIQUE INDEX uk_wb_print_unresolved ON wb_print_attempt(ticket_id) WHERE closed_at IS NULL AND status IN ('initiated','unknown');
COMMIT;
PRAGMA foreign_keys=ON;

-- +goose Down
-- Keep widened constraints to preserve Agent history on rollback.
SELECT 1;
