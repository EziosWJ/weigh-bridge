-- +goose Up
-- ADR 0033: code-based local identity. Site and scale codes replace the
-- numeric ids in every business, grant and device row. Historical rows are
-- migrated in place from the existing wb_site/wb_scale codes; nothing is
-- deleted or renumbered.
--
-- Order matters: every new code column is backfilled while the numeric ids
-- still exist; constraints and the id columns are removed only afterwards.

-- 1. Site code becomes the primary identity of the singleton row.
ALTER TABLE wb_site RENAME COLUMN code TO site_code;

-- 2. Backfill code columns everywhere the numeric ids were used.
ALTER TABLE wb_scale ADD COLUMN site_code VARCHAR(40);
UPDATE wb_scale sc SET site_code = s.site_code FROM wb_site s WHERE s.id = sc.site_id;

ALTER TABLE wb_transport_task ADD COLUMN site_code VARCHAR(40), ADD COLUMN scale_code VARCHAR(40);
UPDATE wb_transport_task t SET site_code = s.site_code, scale_code = sc.code FROM wb_scale sc JOIN wb_site s ON s.id = sc.site_id WHERE sc.id = t.scale_id;
ALTER TABLE wb_ticket ADD COLUMN site_code VARCHAR(40);
UPDATE wb_ticket k SET site_code = s.site_code FROM wb_site s WHERE s.id = k.site_id;
ALTER TABLE wb_sample ADD COLUMN scale_code VARCHAR(40);
UPDATE wb_sample x SET scale_code = sc.code FROM wb_scale sc WHERE sc.id = x.scale_id;
ALTER TABLE wb_weigh_record ADD COLUMN scale_code VARCHAR(40);
UPDATE wb_weigh_record x SET scale_code = sc.code FROM wb_scale sc WHERE sc.id = x.scale_id;
ALTER TABLE wb_scale_session ADD COLUMN scale_code VARCHAR(40);
UPDATE wb_scale_session x SET scale_code = sc.code FROM wb_scale sc WHERE sc.id = x.scale_id;
ALTER TABLE wb_weighing_image ADD COLUMN scale_code VARCHAR(40);
UPDATE wb_weighing_image x SET scale_code = sc.code FROM wb_scale sc WHERE sc.id = x.scale_id;
ALTER TABLE wb_agent_config_backup ADD COLUMN scale_code VARCHAR(40);
UPDATE wb_agent_config_backup x SET scale_code = sc.code FROM wb_scale sc WHERE sc.id = x.scale_id;
ALTER TABLE wb_agent_runtime ADD COLUMN scale_code VARCHAR(40);
UPDATE wb_agent_runtime x SET scale_code = sc.code FROM wb_scale sc WHERE sc.id = x.scale_id;
ALTER TABLE wb_agent_binding ADD COLUMN scale_code VARCHAR(40);
UPDATE wb_agent_binding a SET scale_code = sc.code FROM wb_scale sc WHERE sc.id = a.scale_id;
ALTER TABLE wb_user_site ADD COLUMN site_code VARCHAR(40);
UPDATE wb_user_site us SET site_code = s.site_code FROM wb_site s WHERE s.id = us.site_id;

ALTER TABLE wb_local_identity ADD COLUMN site_code VARCHAR(40), ADD COLUMN scale_code VARCHAR(40);
UPDATE wb_local_identity i SET site_code = s.site_code, scale_code = sc.code FROM wb_site s JOIN wb_scale sc ON sc.site_id = s.id WHERE s.id = i.site_id;

-- Legacy installations upgraded from a pre-sites release can hold seed
-- placeholder scales whose numeric site never existed: schema migrations run
-- before seed migrations in "migrate up --kind all", so wb_scale.site_id from
-- seed 00006 (default 1) may point at a wb_site row that seed 00008 has not
-- created yet. Remove only placeholders that no business or device row
-- references; anything still unresolved aborts with an actionable message
-- rather than a bare NOT NULL violation.
DELETE FROM wb_scale sc
 WHERE sc.site_code IS NULL
   AND NOT EXISTS (SELECT 1 FROM wb_scale_session x WHERE x.scale_id = sc.id)
   AND NOT EXISTS (SELECT 1 FROM wb_agent_binding x WHERE x.scale_id = sc.id)
   AND NOT EXISTS (SELECT 1 FROM wb_agent_runtime x WHERE x.scale_id = sc.id)
   AND NOT EXISTS (SELECT 1 FROM wb_agent_config_backup x WHERE x.scale_id = sc.id)
   AND NOT EXISTS (SELECT 1 FROM wb_transport_task x WHERE x.scale_id = sc.id)
   AND NOT EXISTS (SELECT 1 FROM wb_sample x WHERE x.scale_id = sc.id)
   AND NOT EXISTS (SELECT 1 FROM wb_weigh_record x WHERE x.scale_id = sc.id)
   AND NOT EXISTS (SELECT 1 FROM wb_weighing_image x WHERE x.scale_id = sc.id);
DELETE FROM wb_user_site WHERE site_code IS NULL;
-- +goose StatementBegin
DO $$
BEGIN
 IF EXISTS (SELECT 1 FROM wb_scale WHERE site_code IS NULL)
    OR EXISTS (SELECT 1 FROM wb_scale_session WHERE scale_code IS NULL)
    OR EXISTS (SELECT 1 FROM wb_agent_binding WHERE scale_code IS NULL)
    OR EXISTS (SELECT 1 FROM wb_agent_runtime WHERE scale_code IS NULL)
    OR EXISTS (SELECT 1 FROM wb_agent_config_backup WHERE scale_code IS NULL)
    OR EXISTS (SELECT 1 FROM wb_transport_task WHERE site_code IS NULL OR scale_code IS NULL)
    OR EXISTS (SELECT 1 FROM wb_ticket WHERE site_code IS NULL)
    OR EXISTS (SELECT 1 FROM wb_sample WHERE scale_code IS NULL)
    OR EXISTS (SELECT 1 FROM wb_weigh_record WHERE scale_code IS NULL)
    OR EXISTS (SELECT 1 FROM wb_weighing_image WHERE scale_code IS NULL) THEN
  RAISE EXCEPTION '旧数据引用了不存在的厂区或地磅，无法自动回填编码；请先执行 seed 迁移（task db:migrate:seed）或恢复升级前备份后重试';
 END IF;
END $$;
-- +goose StatementEnd

-- A database with business rows but a missing identity fails here loudly
-- instead of inventing one.
ALTER TABLE wb_scale ALTER COLUMN site_code SET NOT NULL;
ALTER TABLE wb_transport_task ALTER COLUMN site_code SET NOT NULL, ALTER COLUMN scale_code SET NOT NULL;
ALTER TABLE wb_ticket ALTER COLUMN site_code SET NOT NULL;
ALTER TABLE wb_sample ALTER COLUMN scale_code SET NOT NULL;
ALTER TABLE wb_weigh_record ALTER COLUMN scale_code SET NOT NULL;
ALTER TABLE wb_scale_session ALTER COLUMN scale_code SET NOT NULL;
ALTER TABLE wb_weighing_image ALTER COLUMN scale_code SET NOT NULL;
ALTER TABLE wb_agent_config_backup ALTER COLUMN scale_code SET NOT NULL;
ALTER TABLE wb_agent_runtime ALTER COLUMN scale_code SET NOT NULL;
ALTER TABLE wb_agent_binding ALTER COLUMN scale_code SET NOT NULL;
ALTER TABLE wb_user_site ALTER COLUMN site_code SET NOT NULL;

-- 3. Drop the old foreign keys, indexes and primary keys.
ALTER TABLE wb_scale DROP CONSTRAINT IF EXISTS wb_scale_site_id_fkey;
ALTER TABLE wb_scale DROP CONSTRAINT IF EXISTS wb_scale_pkey CASCADE;
ALTER TABLE wb_scale DROP CONSTRAINT IF EXISTS wb_scale_code_key;
ALTER TABLE wb_transport_task DROP CONSTRAINT IF EXISTS wb_transport_task_site_id_fkey;
ALTER TABLE wb_transport_task DROP CONSTRAINT IF EXISTS wb_transport_task_scale_id_fkey;
ALTER TABLE wb_ticket DROP CONSTRAINT IF EXISTS wb_ticket_site_id_fkey;
ALTER TABLE wb_sample DROP CONSTRAINT IF EXISTS wb_sample_scale_id_fkey;
ALTER TABLE wb_weigh_record DROP CONSTRAINT IF EXISTS wb_weigh_record_scale_id_fkey;
ALTER TABLE wb_scale_session DROP CONSTRAINT IF EXISTS wb_scale_session_scale_id_fkey;
ALTER TABLE wb_scale_session DROP CONSTRAINT IF EXISTS wb_scale_session_pkey;
ALTER TABLE wb_weighing_image DROP CONSTRAINT IF EXISTS wb_weighing_image_scale_id_fkey;
ALTER TABLE wb_agent_config_backup DROP CONSTRAINT IF EXISTS wb_agent_config_backup_scale_id_fkey;
ALTER TABLE wb_agent_runtime DROP CONSTRAINT IF EXISTS wb_agent_runtime_scale_id_fkey;
ALTER TABLE wb_agent_runtime DROP CONSTRAINT IF EXISTS wb_agent_runtime_pkey;
ALTER TABLE wb_agent_binding DROP CONSTRAINT IF EXISTS wb_agent_binding_scale_id_fkey;
ALTER TABLE wb_agent_binding DROP CONSTRAINT IF EXISTS wb_agent_binding_pkey;
ALTER TABLE wb_user_site DROP CONSTRAINT IF EXISTS wb_user_site_site_id_fkey;
ALTER TABLE wb_user_site DROP CONSTRAINT IF EXISTS wb_user_site_pkey;
ALTER TABLE wb_local_identity DROP CONSTRAINT IF EXISTS wb_local_identity_site_id_fkey;
ALTER TABLE wb_local_identity DROP CONSTRAINT IF EXISTS wb_local_identity_site_id_key;
ALTER TABLE wb_site DROP CONSTRAINT IF EXISTS wb_site_pkey CASCADE;

DROP INDEX IF EXISTS idx_wb_scale_site;
DROP INDEX IF EXISTS idx_wb_task_site;
DROP INDEX IF EXISTS idx_wb_ticket_site;
DROP INDEX IF EXISTS uk_wb_task_active_plate;
DROP INDEX IF EXISTS idx_wb_sample_scale_time;
DROP INDEX IF EXISTS idx_wb_image_scale_time;
DROP INDEX IF EXISTS idx_wb_agent_backup_scale_time;

ALTER TABLE wb_site DROP COLUMN id;
ALTER TABLE wb_scale DROP COLUMN id, DROP COLUMN site_id;
ALTER TABLE wb_scale_session DROP COLUMN scale_id;
ALTER TABLE wb_transport_task DROP COLUMN site_id, DROP COLUMN scale_id;
ALTER TABLE wb_ticket DROP COLUMN site_id;
ALTER TABLE wb_sample DROP COLUMN scale_id;
ALTER TABLE wb_weigh_record DROP COLUMN scale_id;
ALTER TABLE wb_weighing_image DROP COLUMN scale_id;
ALTER TABLE wb_agent_config_backup DROP COLUMN scale_id;
ALTER TABLE wb_agent_runtime DROP COLUMN scale_id;
ALTER TABLE wb_agent_binding DROP COLUMN scale_id;
ALTER TABLE wb_user_site DROP COLUMN site_id;
ALTER TABLE wb_local_identity DROP COLUMN site_id;

-- 4. Build the code identity: primary keys, foreign keys and indexes.
ALTER TABLE wb_site ADD CONSTRAINT wb_site_pkey PRIMARY KEY (site_code);
ALTER TABLE wb_site DROP CONSTRAINT IF EXISTS wb_site_code_key;
ALTER TABLE wb_scale ADD CONSTRAINT wb_scale_pkey PRIMARY KEY (site_code, code);
-- wb_scale.code stays globally unique: device tables reference a scale by
-- code and one local database belongs to exactly one site.
ALTER TABLE wb_scale ADD CONSTRAINT wb_scale_code_key UNIQUE (code);
ALTER TABLE wb_scale ADD CONSTRAINT fk_wb_scale_site FOREIGN KEY (site_code) REFERENCES wb_site(site_code) ON DELETE RESTRICT;

ALTER TABLE wb_scale_session ADD CONSTRAINT wb_scale_session_pkey PRIMARY KEY (scale_code);
ALTER TABLE wb_scale_session ADD CONSTRAINT fk_wb_session_scale FOREIGN KEY (scale_code) REFERENCES wb_scale(code) ON DELETE RESTRICT;
ALTER TABLE wb_agent_binding ADD CONSTRAINT wb_agent_binding_pkey PRIMARY KEY (scale_code);
ALTER TABLE wb_agent_binding ADD CONSTRAINT fk_wb_binding_scale FOREIGN KEY (scale_code) REFERENCES wb_scale(code) ON DELETE RESTRICT;
ALTER TABLE wb_agent_runtime ADD CONSTRAINT wb_agent_runtime_pkey PRIMARY KEY (scale_code);
ALTER TABLE wb_agent_runtime ADD CONSTRAINT fk_wb_runtime_scale FOREIGN KEY (scale_code) REFERENCES wb_scale(code) ON DELETE RESTRICT;

ALTER TABLE wb_transport_task ADD CONSTRAINT fk_wb_task_site FOREIGN KEY (site_code) REFERENCES wb_site(site_code) ON DELETE RESTRICT;
ALTER TABLE wb_transport_task ADD CONSTRAINT fk_wb_task_scale FOREIGN KEY (site_code, scale_code) REFERENCES wb_scale(site_code, code) ON DELETE RESTRICT;
ALTER TABLE wb_ticket ADD CONSTRAINT fk_wb_ticket_site FOREIGN KEY (site_code) REFERENCES wb_site(site_code) ON DELETE RESTRICT;
ALTER TABLE wb_sample ADD CONSTRAINT fk_wb_sample_scale FOREIGN KEY (scale_code) REFERENCES wb_scale(code) ON DELETE RESTRICT;
ALTER TABLE wb_weigh_record ADD CONSTRAINT fk_wb_record_scale FOREIGN KEY (scale_code) REFERENCES wb_scale(code) ON DELETE RESTRICT;
ALTER TABLE wb_weighing_image ADD CONSTRAINT fk_wb_image_scale FOREIGN KEY (scale_code) REFERENCES wb_scale(code) ON DELETE RESTRICT;
ALTER TABLE wb_agent_config_backup ADD CONSTRAINT fk_wb_backup_scale FOREIGN KEY (scale_code) REFERENCES wb_scale(code) ON DELETE RESTRICT;
ALTER TABLE wb_user_site ADD CONSTRAINT wb_user_site_pkey PRIMARY KEY (user_id, site_code);
ALTER TABLE wb_user_site ADD CONSTRAINT fk_wb_user_site_site FOREIGN KEY (site_code) REFERENCES wb_site(site_code) ON DELETE CASCADE;
ALTER TABLE wb_local_identity ADD CONSTRAINT fk_wb_identity_site FOREIGN KEY (site_code) REFERENCES wb_site(site_code) ON DELETE RESTRICT;

CREATE INDEX idx_wb_scale_site ON wb_scale(site_code);
CREATE INDEX idx_wb_task_site ON wb_transport_task(site_code,create_time,id);
CREATE UNIQUE INDEX uk_wb_task_active_plate ON wb_transport_task(site_code,plate) WHERE status='awaiting_second';
CREATE INDEX idx_wb_ticket_site ON wb_ticket(site_code,issued_at,id);
CREATE INDEX idx_wb_sample_scale_time ON wb_sample(scale_code,captured_at,id);
CREATE INDEX idx_wb_image_scale_time ON wb_weighing_image(scale_code,captured_at,id);
CREATE INDEX idx_wb_agent_backup_scale_time ON wb_agent_config_backup(scale_code,applied_at DESC,id DESC);

-- 5. Daily number sequences become per-site; existing material sequences keep
-- their values and belong to the local site.
ALTER TABLE wb_number_sequence ADD COLUMN site_code VARCHAR(40) NOT NULL DEFAULT '';
-- A pre-sites database may already hold material counters while wb_site is
-- still empty; keeping '' leaves those rows harmless and first-time identity
-- configuration writes new per-site keys (the code floor scan still protects
-- already issued numbers).
UPDATE wb_number_sequence SET site_code = COALESCE((SELECT site_code FROM wb_site LIMIT 1), '') WHERE site_code = '';
ALTER TABLE wb_number_sequence ALTER COLUMN site_code DROP DEFAULT;
ALTER TABLE wb_number_sequence DROP CONSTRAINT wb_number_sequence_pkey;
ALTER TABLE wb_number_sequence ADD PRIMARY KEY (site_code, kind, business_date);
ALTER TABLE wb_number_sequence DROP CONSTRAINT IF EXISTS wb_number_sequence_kind_check;
ALTER TABLE wb_number_sequence ADD CONSTRAINT wb_number_sequence_kind_check CHECK (kind IN ('task', 'ticket', 'test_ticket', 'material', 'party'));

-- +goose Down
-- Codes are the durable identity after this migration; restoring numeric ids
-- requires the database backup taken before the upgrade.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Code identity migration is not automatically reversible; restore the pre-upgrade database backup'; END $$;
-- +goose StatementEnd
