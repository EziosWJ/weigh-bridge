-- +goose Up
INSERT INTO sys_dict_data (dict_type_id, dict_label, dict_value, sort_order, remark)
SELECT t.id, '容重', 'DENSITY', 4, '按容重检测值比例扣减'
FROM sys_dict_type t
WHERE t.dict_code = 'WEIGHING_DEDUCTION_CATEGORY'
ON CONFLICT (dict_type_id, dict_value) DO NOTHING;
UPDATE sys_dict_data d SET deleted = 0, dict_label = '容重', update_time = CURRENT_TIMESTAMP
FROM sys_dict_type t
WHERE d.dict_type_id = t.id AND t.dict_code = 'WEIGHING_DEDUCTION_CATEGORY'
  AND d.dict_value = 'DENSITY' AND d.deleted <> 0;

-- +goose Down
-- Dictionary additions are retained so rollback cannot remove local configuration.
SELECT 1;
