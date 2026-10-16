-- +goose Up
CREATE TABLE sys_notification_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title VARCHAR(200) NOT NULL,
    content TEXT NOT NULL,
    jump_path VARCHAR(1000) NOT NULL DEFAULT '',
    source_type VARCHAR(30) NOT NULL DEFAULT 'MANUAL',
    publisher_id INTEGER,
    publish_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ck_sys_notification_title_not_blank CHECK (length(trim(title)) > 0),
    CONSTRAINT ck_sys_notification_content_not_blank CHECK (length(trim(content)) > 0),
    CONSTRAINT ck_sys_notification_source CHECK (source_type IN ('MANUAL', 'ROLE_CHANGE', 'BUSINESS'))
);

CREATE TABLE sys_notification_recipient_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    notification_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    is_read INTEGER NOT NULL DEFAULT 0,
    read_time DATETIME,
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_sys_notification_recipient_new UNIQUE (notification_id, user_id),
    CONSTRAINT fk_notification_recipient_notification FOREIGN KEY (notification_id) REFERENCES sys_notification_new (id) ON DELETE CASCADE,
    CONSTRAINT fk_notification_recipient_user FOREIGN KEY (user_id) REFERENCES sys_user (id) ON DELETE RESTRICT,
    CONSTRAINT ck_sys_notification_recipient_new_read CHECK (is_read IN (0, 1))
);

INSERT INTO sys_notification_new (id,title,content,source_type,publisher_id,publish_time,create_time,jump_path) SELECT id,title,content,source_type,publisher_id,publish_time,create_time,jump_path FROM sys_notification;
INSERT INTO sys_notification_recipient_new (id,notification_id,user_id,is_read,read_time,create_time) SELECT id,notification_id,user_id,is_read,read_time,create_time FROM sys_notification_recipient;
DROP TABLE sys_notification_recipient;
DROP TABLE sys_notification;
ALTER TABLE sys_notification_new RENAME TO sys_notification;
ALTER TABLE sys_notification_recipient_new RENAME TO sys_notification_recipient;
CREATE INDEX idx_sys_notification_publish_time ON sys_notification (publish_time DESC, id DESC);
CREATE INDEX idx_sys_notification_recipient_user_read ON sys_notification_recipient (user_id, is_read, notification_id DESC);


-- +goose Down
CREATE TABLE sys_notification_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title VARCHAR(200) NOT NULL,
    content TEXT NOT NULL,
    jump_path VARCHAR(1000) NOT NULL DEFAULT '',
    source_type VARCHAR(30) NOT NULL DEFAULT 'MANUAL',
    publisher_id INTEGER,
    publish_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ck_sys_notification_title_not_blank CHECK (length(trim(title)) > 0),
    CONSTRAINT ck_sys_notification_content_not_blank CHECK (length(trim(content)) > 0),
    CONSTRAINT ck_sys_notification_source CHECK (source_type IN ('MANUAL', 'ROLE_CHANGE'))
);

CREATE TABLE sys_notification_recipient_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    notification_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    is_read INTEGER NOT NULL DEFAULT 0,
    read_time DATETIME,
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_sys_notification_recipient_new UNIQUE (notification_id, user_id),
    CONSTRAINT fk_notification_recipient_notification FOREIGN KEY (notification_id) REFERENCES sys_notification_new (id) ON DELETE CASCADE,
    CONSTRAINT fk_notification_recipient_user FOREIGN KEY (user_id) REFERENCES sys_user (id) ON DELETE RESTRICT,
    CONSTRAINT ck_sys_notification_recipient_new_read CHECK (is_read IN (0, 1))
);

INSERT INTO sys_notification_new (id,title,content,source_type,publisher_id,publish_time,create_time,jump_path) SELECT id,title,content,CASE WHEN source_type='BUSINESS' THEN 'MANUAL' ELSE source_type END,publisher_id,publish_time,create_time,jump_path FROM sys_notification;
INSERT INTO sys_notification_recipient_new (id,notification_id,user_id,is_read,read_time,create_time) SELECT id,notification_id,user_id,is_read,read_time,create_time FROM sys_notification_recipient;
DROP TABLE sys_notification_recipient;
DROP TABLE sys_notification;
ALTER TABLE sys_notification_new RENAME TO sys_notification;
ALTER TABLE sys_notification_recipient_new RENAME TO sys_notification_recipient;
CREATE INDEX idx_sys_notification_publish_time ON sys_notification (publish_time DESC, id DESC);
CREATE INDEX idx_sys_notification_recipient_user_read ON sys_notification_recipient (user_id, is_read, notification_id DESC);
