import { Eye, Pencil, Plus, RefreshCw, RotateCcw, Search } from "lucide-react";
import { useMemo, useState } from "react";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { StatusTag } from "@/components/common/status-tag";
import { TableActions } from "@/components/common/table-actions";
import { TableToolbar } from "@/components/common/table-toolbar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import type { DataTableColumn } from "@/types";

type DemoListStatus = "enabled" | "disabled" | "pending";

type DemoListRecord = {
  id: number;
  name: string;
  code: string;
  owner: string;
  department: string;
  status: DemoListStatus;
  updatedAt: string;
};

const records: DemoListRecord[] = [
  {
    id: 1001,
    name: "客户资料同步",
    code: "customer_sync",
    owner: "陈晨",
    department: "客户运营部",
    status: "enabled",
    updatedAt: "2026-05-01 09:30:00",
  },
  {
    id: 1002,
    name: "审批时效提醒",
    code: "approval_sla_notice",
    owner: "李响",
    department: "流程平台部",
    status: "pending",
    updatedAt: "2026-04-30 16:42:00",
  },
  {
    id: 1003,
    name: "归档任务清理",
    code: "archive_cleanup",
    owner: "周宁",
    department: "系统运维部",
    status: "disabled",
    updatedAt: "2026-04-28 11:18:00",
  },
];

const statusMeta: Record<
  DemoListStatus,
  { label: string; tone: "success" | "warning" | "neutral" }
> = {
  enabled: { label: "启用", tone: "success" },
  pending: { label: "待确认", tone: "warning" },
  disabled: { label: "停用", tone: "neutral" },
};

export function ListDemoPage() {
  const [keyword, setKeyword] = useState("");
  const [status, setStatus] = useState<DemoListStatus | "all">("all");
  const [appliedKeyword, setAppliedKeyword] = useState("");
  const [appliedStatus, setAppliedStatus] =
    useState<DemoListStatus | "all">("all");
  const filteredRecords = useMemo(
    () =>
      records.filter((record) => {
        const matchedKeyword =
          !appliedKeyword.trim() ||
          record.name.includes(appliedKeyword.trim()) ||
          record.code.includes(appliedKeyword.trim());
        const matchedStatus =
          appliedStatus === "all" || record.status === appliedStatus;

        return matchedKeyword && matchedStatus;
      }),
    [appliedKeyword, appliedStatus],
  );

  const columns: DataTableColumn<DemoListRecord>[] = [
    {
      title: "业务名称",
      key: "name",
      width: 260,
      render: (_, record) => (
        <div>
          <div className="font-medium text-text-primary">{record.name}</div>
          <div className="text-xs text-text-tertiary">
            {record.code} · ID {record.id}
          </div>
        </div>
      ),
    },
    { title: "负责人", dataIndex: "owner", width: 120 },
    { title: "所属部门", dataIndex: "department", width: 180 },
    {
      title: "状态",
      dataIndex: "status",
      width: 100,
      render: (value) => {
        const meta = statusMeta[value as DemoListStatus];
        return <StatusTag tone={meta.tone}>{meta.label}</StatusTag>;
      },
    },
    {
      title: "更新时间",
      dataIndex: "updatedAt",
      width: 180,
      render: (value) => (
        <span className="whitespace-nowrap tabular-nums">
          {String(value || "-")}
        </span>
      ),
    },
    {
      title: "操作",
      key: "actions",
      nowrap: true,
      align: "center",
      width: 180,
      render: () => (
        <TableActions>
          <Button size="sm" variant="ghost">
            <Eye className="h-4 w-4" aria-hidden />
            查看
          </Button>
          <Button size="sm" variant="ghost">
            <Pencil className="h-4 w-4" aria-hidden />
            编辑
          </Button>
        </TableActions>
      ),
    },
  ];

  return (
    <>
      <PageHeader
        title="标准列表页 Demo"
        description="标准后台列表页结构示例，包含筛选区、工具栏、表格、状态标签、行操作和分页。"
        actions={
          <Button variant="primary">
            <Plus className="h-4 w-4" aria-hidden />
            新建记录
          </Button>
        }
      />

      <SearchFilterBar
        actions={
          <>
            <Button
              variant="secondary"
              onClick={() => {
                setKeyword("");
                setStatus("all");
                setAppliedKeyword("");
                setAppliedStatus("all");
              }}
            >
              <RotateCcw className="h-4 w-4" aria-hidden />
              重置
            </Button>
            <Button
              variant="primary"
              onClick={() => {
                setAppliedKeyword(keyword);
                setAppliedStatus(status);
              }}
            >
              <Search className="h-4 w-4" aria-hidden />
              查询
            </Button>
          </>
        }
      >
        <Input
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
          placeholder="搜索名称或编码"
        />
        <Select
          value={status}
          onChange={(event) =>
            setStatus(event.target.value as DemoListStatus | "all")
          }
          aria-label="筛选状态"
        >
          <option value="all">全部状态</option>
          <option value="enabled">启用</option>
          <option value="pending">待确认</option>
          <option value="disabled">停用</option>
        </Select>
      </SearchFilterBar>

      <DataTableCard>
        <TableToolbar
          title="标准列表"
          description={`共 ${filteredRecords.length} 条数据。`}
          actions={
            <Button size="sm" variant="secondary">
              <RefreshCw className="h-4 w-4" aria-hidden />
              刷新
            </Button>
          }
        />
        <DataTable<DemoListRecord>
          columns={columns}
          dataSource={filteredRecords}
          rowKey="id"
          minWidth={1040}
          empty={<EmptyState title="暂无数据" description="调整筛选条件后重试。" />}
        />
        <Pagination
          page={1}
          pageSize={10}
          total={filteredRecords.length}
          onPageChange={() => undefined}
          onPageSizeChange={() => undefined}
        />
      </DataTableCard>
    </>
  );
}
