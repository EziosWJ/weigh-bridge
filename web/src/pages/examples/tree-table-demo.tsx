import { Plus, RefreshCw, RotateCcw, Search } from "lucide-react";
import { useMemo, useState } from "react";
import { ContentCard } from "@/components/common/content-card";
import { DataTable } from "@/components/common/data-table";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { StatusTag } from "@/components/common/status-tag";
import { TableToolbar } from "@/components/common/table-toolbar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { DataTableColumn } from "@/types";

type Category = {
  id: number;
  name: string;
  code: string;
};

type MemberRecord = {
  id: number;
  name: string;
  account: string;
  categoryId: number;
  role: string;
  status: "enabled" | "disabled";
  lastActive: string;
};

const categories: Category[] = [
  { id: 1, name: "全部组织", code: "all" },
  { id: 11, name: "产品研发中心", code: "rd" },
  { id: 12, name: "客户运营中心", code: "ops" },
  { id: 13, name: "系统运维部", code: "it" },
];

const members: MemberRecord[] = [
  {
    id: 1,
    name: "周宁",
    account: "zhouning",
    categoryId: 11,
    role: "产品负责人",
    status: "enabled",
    lastActive: "2026-05-02 18:20:00",
  },
  {
    id: 2,
    name: "陈晨",
    account: "chenchen",
    categoryId: 12,
    role: "运营专员",
    status: "enabled",
    lastActive: "2026-05-02 16:45:00",
  },
  {
    id: 3,
    name: "林远",
    account: "linyuan",
    categoryId: 13,
    role: "运维工程师",
    status: "disabled",
    lastActive: "2026-04-29 09:12:00",
  },
];

export function TreeTableDemoPage() {
  const [selectedCategoryId, setSelectedCategoryId] = useState(1);
  const [keyword, setKeyword] = useState("");
  const currentCategory = categories.find((item) => item.id === selectedCategoryId);
  const filteredMembers = useMemo(
    () =>
      members.filter((member) => {
        const matchedCategory =
          selectedCategoryId === 1 || member.categoryId === selectedCategoryId;
        const matchedKeyword =
          !keyword.trim() ||
          member.name.includes(keyword.trim()) ||
          member.account.includes(keyword.trim());

        return matchedCategory && matchedKeyword;
      }),
    [keyword, selectedCategoryId],
  );

  const columns: DataTableColumn<MemberRecord>[] = [
    {
      title: "成员",
      key: "member",
      width: 220,
      render: (_, record) => (
        <div>
          <div className="font-medium text-text-primary">{record.name}</div>
          <div className="text-xs text-text-tertiary">{record.account}</div>
        </div>
      ),
    },
    { title: "角色", dataIndex: "role", width: 160 },
    {
      title: "状态",
      dataIndex: "status",
      width: 100,
      render: (value) =>
        value === "enabled" ? (
          <StatusTag tone="success">启用</StatusTag>
        ) : (
          <StatusTag tone="neutral">停用</StatusTag>
        ),
    },
    {
      title: "最近活跃",
      dataIndex: "lastActive",
      width: 180,
      render: (value) => (
        <span className="whitespace-nowrap tabular-nums">
          {String(value || "-")}
        </span>
      ),
    },
  ];

  return (
    <>
      <PageHeader
        title="左树右表 Demo"
        description="展示左侧组织树与右侧列表联动的常见后台页面结构。"
        actions={
          <Button variant="primary">
            <Plus className="h-4 w-4" aria-hidden />
            新增成员
          </Button>
        }
      />

      <div className="grid gap-6 xl:grid-cols-[280px_minmax(0,1fr)]">
        <ContentCard title="组织分类" bodyClassName="p-2">
          <div className="space-y-1">
            {categories.map((category) => (
              <button
                key={category.id}
                type="button"
                className={`flex h-9 w-full items-center justify-between rounded-lg px-3 text-left text-sm transition-colors ${
                  selectedCategoryId === category.id
                    ? "bg-blue-50 text-primary"
                    : "text-text-secondary hover:bg-slate-50 hover:text-text-primary"
                }`}
                onClick={() => setSelectedCategoryId(category.id)}
              >
                <span>{category.name}</span>
                <span className="text-xs text-text-tertiary">{category.code}</span>
              </button>
            ))}
          </div>
        </ContentCard>

        <div>
          <SearchFilterBar
            actions={
              <>
                <Button
                  variant="secondary"
                  onClick={() => {
                    setKeyword("");
                    setSelectedCategoryId(1);
                  }}
                >
                  <RotateCcw className="h-4 w-4" aria-hidden />
                  重置
                </Button>
                <Button variant="primary">
                  <Search className="h-4 w-4" aria-hidden />
                  查询
                </Button>
              </>
            }
          >
            <Input
              value={keyword}
              onChange={(event) => setKeyword(event.target.value)}
              placeholder="搜索姓名或账号"
            />
          </SearchFilterBar>

          <section className="rounded-admin border border-border bg-surface shadow-admin">
            <TableToolbar
              title={currentCategory?.name ?? "成员列表"}
              description={`当前显示 ${filteredMembers.length} 条成员数据。`}
              actions={
                <Button size="sm" variant="secondary">
                  <RefreshCw className="h-4 w-4" aria-hidden />
                  刷新
                </Button>
              }
            />
            <DataTable<MemberRecord>
              columns={columns}
              dataSource={filteredMembers}
              rowKey="id"
              minWidth={760}
            />
            <Pagination
              page={1}
              pageSize={10}
              total={filteredMembers.length}
              onPageChange={() => undefined}
              onPageSizeChange={() => undefined}
            />
          </section>
        </div>
      </div>
    </>
  );
}
