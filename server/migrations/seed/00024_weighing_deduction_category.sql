-- +goose Up
INSERT INTO sys_dict_type (dict_name, dict_code, status, sort_order, is_builtin, remark)
SELECT '净重调整类别', 'WEIGHING_DEDUCTION_CATEGORY', 1, 10, 1, '称重出单净重扣减或上调的原因'
WHERE NOT EXISTS (
    SELECT 1 FROM sys_dict_type WHERE dict_code = 'WEIGHING_DEDUCTION_CATEGORY'
);

INSERT INTO sys_dict_data (dict_type_id, dict_label, dict_value, sort_order, remark)
SELECT t.id, v.label, v.value, v.sort_order, NULL
FROM sys_dict_type t
CROSS JOIN (
    VALUES
        ('水分大', 'WATER', 1),
        ('质量差', 'QUALITY', 2),
        ('杂质多', 'IMPURITY', 3),
        ('其他', 'OTHER', 4)
) AS v(label, value, sort_order)
WHERE t.dict_code = 'WEIGHING_DEDUCTION_CATEGORY'
ON CONFLICT (dict_type_id, dict_value) DO NOTHING;

-- +goose Down
DELETE FROM sys_dict_data
WHERE dict_type_id IN (
    SELECT id FROM sys_dict_type WHERE dict_code = 'WEIGHING_DEDUCTION_CATEGORY'
);
DELETE FROM sys_dict_type
WHERE dict_code = 'WEIGHING_DEDUCTION_CATEGORY' AND is_builtin = 1;
