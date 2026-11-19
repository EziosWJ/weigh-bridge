import { StatusTag } from "@/components/common/status-tag";
import { TableActionMenu, TableActions } from "@/components/common/table-actions";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { formatDateOnly } from "@/lib/datetime";
import type { ApiStatus, DataTableColumn, RoleListRecord } from "@/types";

type RoleColumnsHandlers = {
  onEdit: (role: RoleListRecord) => void;
  onAssignMenus: (role: RoleListRecord) => void;
  onToggleStatus: (role: RoleListRecord, status: ApiStatus) => void;
  onDelete: (role: RoleListRecord) => void;
  selectedIds: Set<number>;
  onToggleSelect: (id: number, checked: boolean) => void;
  onToggleSelectAll: (checked: boolean) => void;
  allSelectableChecked: boolean;
  selectableCount: number;
};

const statusMeta: Record<ApiStatus, { label: string; tone: "success" | "neutral" }> = {
  1: { label: "启用", tone: "success" },
  0: { label: "禁用", tone: "neutral" },
};

export function createRoleColumns({
  onEdit,
  onAssignMenus,
  onToggleStatus,
  onDelete,
  selectedIds,
  onToggleSelect,
  onToggleSelectAll,
  allSelectableChecked,
  selectableCount,
}: RoleColumnsHandlers): DataTableColumn<RoleListRecord>[] {
  return [
    {
      title: (
        <Checkbox
          aria-label="选择当前页普通角色"
          checked={allSelectableChecked}
          disabled={selectableCount === 0}
          onChange={(event) => onToggleSelectAll(event.target.checked)}
        />
      ),
      key: "selection",
      align: "center",
      width: 54,
      render: (_, role) => {
        const disabled = role.isBuiltin === 1;
        return (
          <Checkbox
            aria-label={`选择角色 ${role.roleName}`}
            checked={selectedIds.has(role.id)}
            disabled={disabled}
            title={disabled ? "内置角色不参与批量删除" : undefined}
            onChange={(event) => onToggleSelect(role.id, event.target.checked)}
          />
        );
      },
    },
    {
      title: "角色名称",
      key: "role",
      width: 168,
      render: (_, role) => (
        <div>
          <div className="font-medium text-text-primary">{role.roleName}</div>
          <div className="text-xs text-text-tertiary">
            {role.roleCode} · ID {role.id}
          </div>
        </div>
      ),
    },
    {
      title: "角色编码",
      dataIndex: "roleCode",
      width: 146,
    },
    {
      title: "状态",
      dataIndex: "status",
      width: 84,
      render: (value) => {
        const status = value as ApiStatus;
        const meta = statusMeta[status];
        return <StatusTag tone={meta.tone}>{meta.label}</StatusTag>;
      },
    },
    {
      title: "属性",
      dataIndex: "isBuiltin",
      width: 84,
      render: (value) =>
        value === 1 ? (
          <StatusTag tone="info">内置</StatusTag>
        ) : (
          <StatusTag tone="neutral">普通</StatusTag>
        ),
    },
    {
      title: "排序",
      dataIndex: "sortOrder",
      align: "center",
      width: 64,
      render: (value) => (
        <span className="tabular-nums">{String(value ?? 0)}</span>
      ),
    },
    {
      title: "备注",
      dataIndex: "remark",
      width: 160,
      render: (value) => (
        <span className="block max-w-[220px] truncate text-text-secondary" title={typeof value === "string" ? value : undefined}>
          {String(value || "-")}
        </span>
      ),
    },
    {
      title: "创建时间",
      dataIndex: "createTime",
      width: 116,
      render: (value) => (
        <span className="whitespace-nowrap tabular-nums">
          {formatDateOnly(
            typeof value === "string" ? value : value ? String(value) : "",
          )}
        </span>
      ),
    },
    {
      title: "操作",
      key: "actions",
      nowrap: true,
      align: "center",
      width: 242,
      render: (_, role) => (
        <TableActions>
          <Button size="sm" variant="secondary" onClick={() => onEdit(role)}>
            编辑
          </Button>
          <Button size="sm" variant="ghost" onClick={() => onAssignMenus(role)}>
            分配菜单
          </Button>
          <TableActionMenu
            items={[
              {
                label: role.status === 1 ? "禁用" : "启用",
                onSelect: () => onToggleStatus(role, role.status === 1 ? 0 : 1),
                danger: role.status === 1,
              },
              {
                label: "删除",
                onSelect: () => onDelete(role),
                danger: true,
                disabled: role.isBuiltin === 1,
                title: role.isBuiltin === 1 ? "内置角色不允许删除" : undefined,
              },
            ]}
          />
        </TableActions>
      ),
    },
  ];
}
