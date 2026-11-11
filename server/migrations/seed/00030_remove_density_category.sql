-- +goose Up
-- 容重不再作为化验指标或扣减类别。历史单据保留当时的类别名称快照。
UPDATE sys_dict_data d SET deleted = 1, update_time = CURRENT_TIMESTAMP
FROM sys_dict_type t
WHERE d.dict_type_id = t.id AND t.dict_code = 'WEIGHING_DEDUCTION_CATEGORY'
  AND d.dict_value = 'DENSITY' AND d.deleted = 0;

-- +goose Down
-- Dictionary removal is retained; restoring it requires an explicit local decision.
SELECT 1;
