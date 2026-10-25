-- +goose Up
-- Camera module permissions. The visible page entry is added by the Web work
-- package; these codes are frozen here so API, RBAC and pages agree.
INSERT INTO sys_menu (parent_id, menu_name, menu_type, permission_code, sort_order, visible, status, is_builtin)
VALUES
    (0, '摄像头查看', 'MENU', 'weighing:camera:view', 102, 0, 1, 1),
    (0, '摄像头与视频维护', 'MENU', 'weighing:camera:manage', 103, 0, 1, 1)
ON CONFLICT (permission_code) DO NOTHING;

INSERT INTO sys_role_menu (role_id, menu_id)
SELECT r.id, m.id
FROM sys_role r
CROSS JOIN sys_menu m
WHERE r.deleted = 0
  AND m.deleted = 0
  AND r.role_code IN ('ADMIN','WEIGHING_OPERATOR')
  AND m.permission_code IN ('weighing:camera:view','weighing:camera:manage')
  AND (r.role_code = 'ADMIN' OR m.permission_code = 'weighing:camera:view')
ON CONFLICT (role_id, menu_id) DO NOTHING;

INSERT INTO wb_camera_settings (id, config_version, capture_timeout_ms, video_version, video_sync_state)
VALUES (1, 1, 6000, 1, 'pending')
ON CONFLICT (id) DO NOTHING;

-- +goose Down
DELETE FROM wb_camera_settings WHERE id = 1;
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE permission_code IN ('weighing:camera:view','weighing:camera:manage'));
DELETE FROM sys_menu WHERE permission_code IN ('weighing:camera:view','weighing:camera:manage') AND is_builtin=1;
