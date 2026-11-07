-- +goose Up
-- Preserve locally customized labels and historical ticket snapshots.
UPDATE sys_dict_data d SET dict_label = CASE d.dict_value WHEN 'WATER' THEN '水分' ELSE '杂质' END, update_time = CURRENT_TIMESTAMP
FROM sys_dict_type t
WHERE d.dict_type_id = t.id AND t.dict_code = 'WEIGHING_DEDUCTION_CATEGORY'
  AND ((d.dict_value = 'WATER' AND d.dict_label = '水分大') OR (d.dict_value = 'IMPURITY' AND d.dict_label = '杂质多'));
UPDATE sys_dict_data d SET deleted = 1, update_time = CURRENT_TIMESTAMP
FROM sys_dict_type t
WHERE d.dict_type_id = t.id AND t.dict_code = 'WEIGHING_DEDUCTION_CATEGORY'
  AND d.dict_value = 'QUALITY' AND d.dict_label = '质量差' AND d.deleted = 0;

-- +goose Down
-- Dictionary changes are retained so rollback cannot overwrite local edits.
SELECT 1;
