import { Building2, ChevronDown, ChevronRight, FolderTree } from "lucide-react";
import { useState } from "react";
import { ContentCard } from "@/components/common/content-card";
import { PageHeader } from "@/components/common/page-header";
import { StatusTag } from "@/components/common/status-tag";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

type DemoTreeNode = {
  id: number;
  label: string;
  code: string;
  owner: string;
  status: "enabled" | "disabled";
  children?: DemoTreeNode[];
};

const treeData: DemoTreeNode[] = [
  {
    id: 1,
    label: "集团总部",
    code: "HQ",
    owner: "林远",
    status: "enabled",
    children: [
      {
        id: 11,
        label: "产品研发中心",
        code: "RD",
        owner: "周宁",
        status: "enabled",
      },
      {
        id: 12,
        label: "客户运营中心",
        code: "OPS",
        owner: "陈晨",
        status: "enabled",
      },
    ],
  },
  {
    id: 2,
    label: "华东区域",
    code: "EAST",
    owner: "李响",
    status: "enabled",
    children: [
      {
        id: 21,
        label: "上海分部",
        code: "SH",
        owner: "吴越",
        status: "enabled",
      },
      {
        id: 22,
        label: "杭州分部",
        code: "HZ",
        owner: "许诺",
        status: "disabled",
      },
    ],
  },
];

function flatten(nodes: DemoTreeNode[]): DemoTreeNode[] {
  return nodes.flatMap((node) => [node, ...(node.children ? flatten(node.children) : [])]);
}

export function TreeDemoPage() {
  const [selectedId, setSelectedId] = useState(1);
  const [expandedIds, setExpandedIds] = useState<Set<number>>(
    () => new Set(treeData.map((item) => item.id)),
  );
  const selectedNode = flatten(treeData).find((node) => node.id === selectedId);

  const toggleExpanded = (id: number) => {
    setExpandedIds((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  return (
    <>
      <PageHeader
        title="树形列表 Demo"
        description="展示部门树、菜单树、分类树等场景的基础树形交互和右侧内容联动。"
      />

      <div className="grid gap-6 lg:grid-cols-[320px_minmax(0,1fr)]">
        <ContentCard title="组织树" description="点击节点查看右侧内容。">
          <div role="tree" aria-label="组织树">
            {treeData.map((node) => (
              <TreeNodeItem
                key={node.id}
                node={node}
                level={0}
                selectedId={selectedId}
                expandedIds={expandedIds}
                onToggle={toggleExpanded}
                onSelect={setSelectedId}
              />
            ))}
          </div>
        </ContentCard>

        <ContentCard
          title={selectedNode?.label ?? "节点详情"}
          description="右侧区域展示当前节点的核心信息和可承载的业务内容。"
          extra={<FolderTree className="h-5 w-5 text-text-tertiary" aria-hidden />}
        >
          <div className="grid gap-4 md:grid-cols-2">
            <InfoItem label="节点编码" value={selectedNode?.code} />
            <InfoItem label="负责人" value={selectedNode?.owner} />
            <InfoItem
              label="状态"
              value={
                selectedNode?.status === "enabled" ? (
                  <StatusTag tone="success">启用</StatusTag>
                ) : (
                  <StatusTag tone="neutral">停用</StatusTag>
                )
              }
            />
            <InfoItem
              label="子节点数量"
              value={String(selectedNode?.children?.length ?? 0)}
            />
          </div>
        </ContentCard>
      </div>
    </>
  );
}

function TreeNodeItem({
  node,
  level,
  selectedId,
  expandedIds,
  onToggle,
  onSelect,
}: {
  node: DemoTreeNode;
  level: number;
  selectedId: number;
  expandedIds: Set<number>;
  onToggle: (id: number) => void;
  onSelect: (id: number) => void;
}) {
  const hasChildren = Boolean(node.children?.length);
  const expanded = expandedIds.has(node.id);
  const selected = selectedId === node.id;

  return (
    <div>
      <div
        className={cn(
          "flex h-9 items-center gap-1 rounded-lg text-sm",
          selected ? "bg-blue-50 text-primary" : "text-text-primary hover:bg-slate-50",
        )}
        style={{ paddingLeft: 8 + level * 16 }}
        role="treeitem"
        aria-selected={selected}
        aria-expanded={hasChildren ? expanded : undefined}
      >
        {hasChildren ? (
          <Button
            size="icon"
            variant="ghost"
            className="h-6 w-6"
            onClick={() => onToggle(node.id)}
            aria-label={expanded ? "收起节点" : "展开节点"}
          >
            {expanded ? (
              <ChevronDown className="h-4 w-4" aria-hidden />
            ) : (
              <ChevronRight className="h-4 w-4" aria-hidden />
            )}
          </Button>
        ) : (
          <span className="h-6 w-6 shrink-0" />
        )}
        <button
          type="button"
          className="flex min-w-0 flex-1 items-center gap-2 px-1 text-left"
          onClick={() => onSelect(node.id)}
        >
          <Building2 className="h-4 w-4 shrink-0" aria-hidden />
          <span className="truncate">{node.label}</span>
        </button>
      </div>
      {hasChildren && expanded && (
        <div role="group">
          {node.children?.map((child) => (
            <TreeNodeItem
              key={child.id}
              node={child}
              level={level + 1}
              selectedId={selectedId}
              expandedIds={expandedIds}
              onToggle={onToggle}
              onSelect={onSelect}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function InfoItem({ label, value }: { label: string; value?: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-border bg-slate-50 px-4 py-3">
      <div className="text-[13px] text-text-tertiary">{label}</div>
      <div className="mt-1 text-sm font-medium text-text-primary">{value ?? "-"}</div>
    </div>
  );
}
