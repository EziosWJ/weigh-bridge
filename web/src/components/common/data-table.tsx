import type { ReactNode } from "react";
import { EmptyState } from "@/components/common/empty-state";
import { cn } from "@/lib/utils";
import type { DataTableColumn } from "@/types";

type DataTableProps<T> = {
  columns: DataTableColumn<T>[];
  dataSource: T[];
  rowKey: keyof T | ((record: T) => string | number);
  loading?: boolean;
  error?: ReactNode;
  empty?: ReactNode;
  minWidth?: number;
  className?: string;
  compact?: boolean;
  stickyHeader?: boolean;
  /** 桌面端固定首尾两列；业务页面将标识列放首位、操作列放末位。 */
  fixedEdges?: boolean;
  ariaLabel?: string;
  onRowClick?: (record: T) => void;
  rowClassName?: (record: T, index: number) => string | undefined;
};

function getRowKey<T>(
  rowKey: DataTableProps<T>["rowKey"],
  record: T,
): string | number {
  if (typeof rowKey === "function") {
    return rowKey(record);
  }

  return String(record[rowKey]);
}

function getColumnKey<T>(column: DataTableColumn<T>, index: number) {
  return column.key ?? String(column.dataIndex ?? index);
}

export function DataTable<T>({
  columns,
  dataSource,
  rowKey,
  loading = false,
  error,
  empty,
  minWidth = 840,
  className,
  compact = false,
  stickyHeader = false,
  fixedEdges = false,
  ariaLabel = "数据表格",
  onRowClick,
  rowClassName,
}: DataTableProps<T>) {
  if (loading) {
    return (
      <div className={cn("space-y-space-3 p-card", className)} aria-busy="true" aria-live="polite">
        {Array.from({ length: 5 }).map((_, index) => (
          <div
            key={index}
            className="h-11 animate-pulse rounded-control bg-neutral-background"
          />
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <div className={cn("p-card", className)}>
        {typeof error === "string" ? (
          <EmptyState title="加载失败" description={error} />
        ) : (
          error
        )}
      </div>
    );
  }

  if (dataSource.length === 0) {
    return (
      <div className={cn("p-card", className)}>
        {empty ?? (
          <EmptyState
            title="暂无数据"
            description="当前没有可展示的数据。"
          />
        )}
      </div>
    );
  }

  return (
    <div
      className={cn("min-w-0 overflow-auto", (stickyHeader || fixedEdges) && "isolate", className)}
      role={stickyHeader || fixedEdges ? "region" : undefined}
      aria-label={stickyHeader || fixedEdges ? ariaLabel : undefined}
      tabIndex={stickyHeader || fixedEdges ? 0 : undefined}
    >
      <table
        className={cn("w-full border-separate border-spacing-0 text-left", compact && "table-fixed")}
        style={{ minWidth }}
      >
        <thead>
          <tr className="bg-neutral-background text-sm text-text-secondary">
            {columns.map((column, index) => (
              <th
                key={getColumnKey(column, index)}
                className={cn(
                  "border-b border-border px-space-5 py-space-3 font-medium",
                  compact && "!px-space-3 !py-space-2",
                  stickyHeader && "sticky top-0 z-20 bg-neutral-background",
                  fixedEdges && index === 0 && "lg:sticky lg:left-0 lg:z-30 lg:bg-neutral-background lg:border-r",
                  fixedEdges && index === columns.length - 1 && "lg:sticky lg:right-0 lg:z-30 lg:bg-neutral-background lg:border-l",
                  column.align === "center" && "text-center",
                  column.align === "right" && "text-right",
                  column.nowrap && "whitespace-nowrap",
                )}
                style={{ width: column.width }}
                scope="col"
              >
                {column.title}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {dataSource.map((record, rowIndex) => (
            <tr
              key={getRowKey(rowKey, record)}
              className={cn(
                "group transition-colors",
                compact && "hover:bg-neutral-background",
                onRowClick && "cursor-pointer hover:bg-neutral-background",
                rowClassName?.(record, rowIndex),
              )}
              onClick={(event) => {
                if (!onRowClick) return;

                const target = event.target as HTMLElement | null;
                if (target?.closest("button, a, input, textarea, select, label")) {
                  return;
                }

                onRowClick(record);
              }}
            >
              {columns.map((column, columnIndex) => {
                const value = column.dataIndex
                  ? record[column.dataIndex]
                  : undefined;

                return (
                  <td
                    key={getColumnKey(column, columnIndex)}
                    className={cn(
                      "border-b border-border px-space-5 py-space-3 align-middle",
                      compact && "break-words !px-space-3 !py-space-2 [overflow-wrap:anywhere]",
                      fixedEdges && columnIndex === 0 && "lg:sticky lg:left-0 lg:z-10 lg:border-r lg:bg-surface lg:group-hover:bg-neutral-background",
                      fixedEdges && columnIndex === columns.length - 1 && "lg:sticky lg:right-0 lg:z-10 lg:border-l lg:bg-surface lg:group-hover:bg-neutral-background",
                      column.align === "center" && "text-center",
                      column.align === "right" && "text-right",
                      column.nowrap && "whitespace-nowrap",
                    )}
                    style={{ width: column.width }}
                  >
                    {column.render
                      ? column.render(value, record, rowIndex)
                      : String(value ?? "")}
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
