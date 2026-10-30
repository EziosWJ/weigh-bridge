import { useId, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { CircleHelp } from 'lucide-react';

/** 补充说明支持鼠标、键盘聚焦和触屏点击。 */
export function HelpHint({ label, children, align = 'left', above = false }: {
  label: string;
  children: string;
  align?: 'left' | 'right';
  above?: boolean;
}) {
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const tooltip = useRef<HTMLSpanElement>(null);
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const [position, setPosition] = useState({ left: 0, top: 0 });
  const open = hovered || focused;
  useLayoutEffect(() => {
    if (!open) return;
    const place = () => {
      if (!trigger.current || !tooltip.current) return;
      const anchor = trigger.current.getBoundingClientRect();
      const box = tooltip.current.getBoundingClientRect();
      const left = Math.max(8, Math.min(align === 'right' ? anchor.right - box.width : anchor.left, window.innerWidth - box.width - 8));
      const preferredTop = above ? anchor.top - box.height : anchor.bottom;
      const top = Math.max(8, Math.min(preferredTop, window.innerHeight - box.height - 8));
      setPosition({ left, top });
    };
    place();
    window.addEventListener('resize', place);
    window.addEventListener('scroll', place, true);
    return () => { window.removeEventListener('resize', place); window.removeEventListener('scroll', place, true); };
  }, [open, align, above]);
  return <span className="inline-flex align-middle">
    <button ref={trigger} type="button" aria-label={label} aria-describedby={id} onMouseEnter={() => setHovered(true)} onMouseLeave={() => setHovered(false)} onFocus={() => setFocused(true)} onBlur={() => setFocused(false)} className="inline-flex h-6 w-6 items-center justify-center rounded text-text-tertiary hover:text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary">
      <CircleHelp className="h-4 w-4" aria-hidden />
    </button>
    {createPortal(<span ref={tooltip} id={id} role="tooltip" style={{ left: position.left, top: position.top, visibility: open ? 'visible' : 'hidden' }} className="pointer-events-none fixed z-50 w-72 max-w-[calc(100vw-16px)] rounded-control border border-border bg-surface p-3 text-left text-xs font-normal leading-relaxed text-text-primary shadow-lg">{children}</span>, document.body)}
  </span>;
}
