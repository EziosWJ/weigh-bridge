import { StatusTag } from "@/components/common/status-tag";
import { TableActionMenu, TableActions } from "@/components/common/table-actions";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { formatDateOnly, formatDateTime } from "@/lib/datetime";
import type { ApiStatus, DataTableColumn, UserRecord } from "@/types";
import { getRoleNames } from "./utils";

const statusMeta: Record<
  ApiStatus,
  { label: string; tone: "success" | "neutral" }
> = {
  1: { label: "启用", tone: "success" },
  0: { label: "禁用", tone: "neutral" },
};

type UserColumnActions = {
  onEdit: (user: UserRecord) => void;
  onAssignRoles: (user: UserRecord) => void;
  onAssignSites:(user:UserRecord)=>void;
  onChangeStatus: (user: UserRecord, status: ApiStatus) => void;
  onResetPassword: (user: UserRecord) => void;
  onDelete: (user: UserRecord) => void;
  selectedIds: Set<number>;
  onToggleSelect: (id: number, checked: boolean) => void;
  onToggleSelectAll: (checked: boolean) => void;
  allSelectableChecked: boolean;
  selectableCount: number;
};

export function createUserColumns({
  onEdit,
  onAssignRoles,
  onAssignSites,
  onChangeStatus,
  onResetPassword,
  onDelete,
  selectedIds,
  onToggleSelect,
  onToggleSelectAll,
  allSelectableChecked,
  selectableCount,
}: UserColumnActions): DataTableColumn<UserRecord>[] {
  return [
    {
      title: (
        <Checkbox
          aria-label="选择当前页普通用户"
          checked={allSelectableChecked}
          disabled={selectableCount === 0}
          onChange={(event) => onToggleSelectAll(event.target.checked)}
        />
      ),
      key: "selection",
      align: "center",
      width: 54,
      render: (_, user) => {
        const disabled = user.isBuiltin === 1;
        return (
          <Checkbox
            aria-label={`选择用户 ${user.nickname || user.username || user.id}`}
            checked={selectedIds.has(user.id)}
            disabled={disabled}
            title={disabled ? "内置用户不参与批量删除" : undefined}
            onChange={(event) => onToggleSelect(user.id, event.target.checked)}
          />
        );
      },
    },
    {
      title: "用户",
      key: "user",
      width: 168,
      render: (_, user) => (
        <div>
          <div className="font-medium text-text-primary">
            {user.nickname || user.username}
          </div>
          <div className="text-xs text-text-tertiary">
            {user.username} · ID {user.id}
          </div>
        </div>
      ),
    },
    {
      title: "联系方式",
      key: "contact",
      width: 180,
      render: (_, user) => (
        <div className="space-y-0.5 text-sm">
          <div>{user.phone || "-"}</div>
          <div className="text-xs text-text-tertiary">{user.email || "-"}</div>
        </div>
      ),
    },
    {
      title: "部门",
      dataIndex: "deptName",
      width: 112,
      render: (value) => String(value || "-"),
    },
    {
      title: "角色",
      key: "roles",
      width: 140,
      render: (_, user) => (
        <span className="line-clamp-2 text-sm" title={getRoleNames(user)}>{getRoleNames(user)}</span>
      ),
    },
    {
      title: "状态",
      dataIndex: "status",
      width: 72,
      render: (value) => {
        const status = value as ApiStatus;
        const meta = statusMeta[status];
        return <StatusTag tone={meta.tone}>{meta.label}</StatusTag>;
      },
    },
    {
      title: "最近登录",
      dataIndex: "lastLoginTime",
      width: 168,
      render: (value) => (
        <span className="whitespace-nowrap tabular-nums">
          {formatDateTime(
            typeof value === "string" ? value : value ? String(value) : "",
          )}
        </span>
      ),
    },
    {
      title: "创建时间",
      dataIndex: "createTime",
      width: 118,
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
      render: (_, user) => (
        <TableActions>
          <Button size="sm" variant="secondary" onClick={() => onEdit(user)}>
            编辑
          </Button>
          <Button size="sm" variant="ghost" onClick={() => onAssignSites(user)}>
            本厂授权
          </Button>
          <TableActionMenu
            items={[
              { label: "分配角色", onSelect: () => onAssignRoles(user) },
              { label: "重置密码", onSelect: () => onResetPassword(user) },
              {
                label: user.status === 1 ? "禁用" : "启用",
                onSelect: () => onChangeStatus(user, user.status === 1 ? 0 : 1),
                danger: user.status === 1,
              },
              {
                label: "删除",
                onSelect: () => onDelete(user),
                danger: true,
                disabled: user.isBuiltin === 1,
                title: user.isBuiltin === 1 ? "内置用户不允许删除" : undefined,
              },
            ]}
          />
        </TableActions>
      ),
    },
  ];
}
