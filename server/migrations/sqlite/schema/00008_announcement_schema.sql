-- +goose Up
ALTER TABLE sys_notification ADD COLUMN jump_path VARCHAR(1000) NOT NULL DEFAULT '';
CREATE TABLE sys_announcement (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title VARCHAR(200) NOT NULL,
    content TEXT NOT NULL,
    scope VARCHAR(20) NOT NULL CHECK (scope IN ('PUBLIC', 'INTERNAL')),
    status VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'PUBLISHED', 'WITHDRAWN')),
    publisher_id BIGINT NOT NULL,
    publish_time DATETIME,
    expires_at DATETIME,
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (length(trim(title)) > 0),
    CHECK (length(trim(content)) > 0)
);
CREATE INDEX idx_announcement_visibility ON sys_announcement (scope, status, expires_at, id);
-- +goose Down
DROP TABLE sys_announcement;
ALTER TABLE sys_notification DROP COLUMN jump_path;
