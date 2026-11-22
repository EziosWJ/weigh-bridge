import { useEffect, useId, useRef, type PropsWithChildren } from "react";
import { ChevronDown, Ellipsis } from "lucide-react";
import { Link } from "react-router-dom";
import { buttonStyles } from "@/components/ui/button-styles";
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
        "inline-flex flex-nowrap items-center gap-2 whitespace-nowrap",
        className,
      )}
    >
      {children}
    </div>
  );
}

type TableActionItem = {
  label: string;
  title?: string;
  danger?: boolean;
  disabled?: boolean;
} & (
  | { to: string; onSelect?: never }
  | { to?: never; onSelect: () => void }
);

export function TableActionMenu({
  items,
  label = "更多操作",
  compact = false,
}: {
  items: TableActionItem[];
  label?: string;
  compact?: boolean;
}) {
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const hasItems = items.length > 0;

  useEffect(() => {
    const element = panel.current;
    if (!element) return;
    function dismiss(event: Event) {
      if (event.target instanceof Node && element?.contains(event.target)) return;
      if (element?.matches(":popover-open")) element.hidePopover();
    }
    const toggle = () => {
      if (element.matches(":popover-open")) {
        window.addEventListener("scroll", dismiss, true);
        window.addEventListener("resize", dismiss);
      } else {
        removeListeners();
      }
    };
    function removeListeners() {
      window.removeEventListener("scroll", dismiss, true);
      window.removeEventListener("resize", dismiss);
    }
    element.addEventListener("toggle", toggle);
    return () => {
      element.removeEventListener("toggle", toggle);
      removeListeners();
    };
  }, [hasItems]);

  if (!hasItems) return null;

  function place() {
    const element = panel.current;
    const anchor = trigger.current?.getBoundingClientRect();
    if (!element?.matches(":popover-open") || !anchor) return;
    const inset = 8;
    const gap = 4;
    const below = window.innerHeight - anchor.bottom - gap - inset;
    const above = anchor.top - gap - inset;
    const openAbove = element.scrollHeight > below && above > below;
    element.style.maxHeight = `${Math.max(0, openAbove ? above : below)}px`;
    element.style.left = `${Math.max(inset, Math.min(anchor.right, window.innerWidth - inset) - element.offsetWidth)}px`;
    element.style.top = `${openAbove ? anchor.top - gap - element.offsetHeight : anchor.bottom + gap}px`;
  }

  function close() {
    panel.current?.hidePopover();
    trigger.current?.focus({ preventScroll: true });
  }

  return (
    <>
      <button
        ref={trigger}
        type="button"
        popoverTarget={id}
        aria-label={label}
        title={compact ? label : undefined}
        className={buttonStyles({ size: "sm", className: compact ? "w-8 px-0" : "px-2" })}
      >
        {compact ? <Ellipsis className="h-4 w-4" aria-hidden /> : <>更多<ChevronDown className="h-3.5 w-3.5" aria-hidden /></>}
      </button>
      <div
        ref={panel}
        id={id}
        popover="auto"
        onToggle={(event) => {
          if (event.newState !== "open") return;
          place();
          event.currentTarget.querySelector<HTMLElement>("a, button")?.focus({ preventScroll: true });
        }}
        onBlur={(event) => {
          if (event.relatedTarget && !event.currentTarget.contains(event.relatedTarget)
            && event.relatedTarget !== trigger.current && event.currentTarget.matches(":popover-open")) {
            event.currentTarget.hidePopover();
          }
        }}
        role="group"
        aria-label={label}
        className="fixed inset-auto m-0 min-w-36 max-w-[calc(100vw-16px)] overflow-auto rounded-control border border-border bg-surface p-1 shadow-floating"
      >
        {items.map(({ label, to, title, danger, disabled, onSelect }) => {
          const className = cn(
            "flex min-h-8 w-full items-center rounded-control px-3 py-1.5 text-left text-sm hover:bg-neutral-background focus-visible:bg-neutral-background",
            danger ? "text-error" : "text-text-primary",
            disabled && "cursor-not-allowed text-text-tertiary",
          );
          if (to && !disabled) return <Link key={label} to={to} title={title} className={className} onClick={close}>{label}</Link>;
          return (
            <button
              key={label}
              type="button"
              title={title}
              aria-disabled={disabled || undefined}
              className={className}
              onClick={() => {
                if (disabled) return;
                close();
                onSelect?.();
              }}
            >
              {label}
            </button>
          );
        })}
      </div>
    </>
  );
}
