import type { PropsWithChildren } from "react";
import { cn } from "@/lib/utils";

type TableActionsProps = PropsWithChildren<{
  className?: string;
}>;

/**
 * 操作列的按钮保持在同一行；表格在窄屏下通过横向滚动保留完整操作。
 */
export function TableActions({ children, className }: TableActionsProps) {
  return (
    <div
      className={cn(
        "inline-flex flex-nowrap items-center justify-center gap-1 whitespace-nowrap",
        className,
      )}
    >
      {children}
    </div>
  );
}
