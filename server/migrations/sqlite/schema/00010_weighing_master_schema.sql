-- +goose Up
CREATE TABLE wb_material_category (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(100) NOT NULL CHECK (length(trim(name)) > 0),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_by INTEGER NOT NULL,
    update_by INTEGER NOT NULL
);
CREATE TABLE wb_material (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    code VARCHAR(40) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL CHECK (length(trim(name)) > 0),
    category_id INTEGER REFERENCES wb_material_category(id) ON DELETE RESTRICT,
    spec VARCHAR(200) NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    remark TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_by INTEGER NOT NULL,
    update_by INTEGER NOT NULL
);
CREATE INDEX idx_wb_material_category ON wb_material(category_id);
CREATE INDEX idx_wb_material_enabled ON wb_material(enabled, id);
CREATE TABLE wb_party (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    code VARCHAR(40) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL CHECK (length(trim(name)) > 0),
    short_name VARCHAR(100) NOT NULL DEFAULT '',
    nature VARCHAR(20) NOT NULL CHECK (nature IN ('enterprise', 'individual')),
    is_supplier INTEGER NOT NULL DEFAULT 0 CHECK (is_supplier IN (0, 1)),
    is_customer INTEGER NOT NULL DEFAULT 0 CHECK (is_customer IN (0, 1)),
    is_carrier INTEGER NOT NULL DEFAULT 0 CHECK (is_carrier IN (0, 1)),
    phone VARCHAR(50) NOT NULL DEFAULT '',
    address VARCHAR(500) NOT NULL DEFAULT '',
    credit_code VARCHAR(50) NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    remark TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_by INTEGER NOT NULL,
    update_by INTEGER NOT NULL,
    CHECK (is_supplier + is_customer + is_carrier > 0)
);
CREATE INDEX idx_wb_party_enabled ON wb_party(enabled, id);
CREATE TABLE wb_party_contact (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    party_id INTEGER NOT NULL REFERENCES wb_party(id) ON DELETE RESTRICT,
    name VARCHAR(100) NOT NULL CHECK (length(trim(name)) > 0),
    phone VARCHAR(50) NOT NULL DEFAULT '',
    purpose VARCHAR(200) NOT NULL DEFAULT '',
    remark TEXT NOT NULL DEFAULT '',
    is_default INTEGER NOT NULL DEFAULT 0 CHECK (is_default IN (0, 1))
);
CREATE INDEX idx_wb_party_contact_party ON wb_party_contact(party_id);
CREATE UNIQUE INDEX uk_wb_party_default_contact ON wb_party_contact(party_id) WHERE is_default = 1;
CREATE TABLE wb_scale (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    code VARCHAR(40) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL CHECK (length(trim(name)) > 0),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1))
);
CREATE TABLE wb_number_sequence (
    kind VARCHAR(30) NOT NULL CHECK (kind IN ('task', 'test_ticket', 'material', 'party')),
    business_date VARCHAR(8) NOT NULL,
    next_value INTEGER NOT NULL CHECK (next_value > 0),
    PRIMARY KEY (kind, business_date)
);

-- +goose Down
DROP TABLE wb_number_sequence;
DROP TABLE wb_scale;
DROP TABLE wb_party_contact;
DROP TABLE wb_party;
DROP TABLE wb_material;
DROP TABLE wb_material_category;
