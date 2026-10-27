-- +goose Up
-- Local operation access belongs to the account, not to a mutable site code.
-- Preserve only grants for the configured local identity; absence remains denial.
CREATE TABLE wb_local_user_access (
    user_id BIGINT PRIMARY KEY REFERENCES sys_user(id) ON DELETE CASCADE,
    can_operate SMALLINT NOT NULL CHECK (can_operate IN (0, 1))
);
INSERT INTO wb_local_user_access (user_id, can_operate)
SELECT g.user_id, g.can_operate
FROM wb_user_site g
JOIN wb_local_identity i ON i.id = 1 AND i.site_code = g.site_code;

-- +goose Down
-- Restoring an older binary requires a coordinated database backup restore;
-- legacy grants are no longer kept current after the authorization cutover.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Local user access migration is not automatically reversible; restore the pre-upgrade database backup'; END $$;
-- +goose StatementEnd
