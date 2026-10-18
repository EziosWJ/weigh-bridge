import { useId, useState, type ReactNode } from 'react';
import { cn } from '@/lib/utils';

export function SectionTabs({ label, items, value, onValueChange }: {
  label: string;
  items: { value: string; label: string; content: ReactNode }[];
  value?: string;
  onValueChange?: (value: string) => void;
}) {
  const id = useId();
  const [localValue, setLocalValue] = useState(items[0]?.value);
  const selected = value ?? localValue;
  const select = (next: string) => { setLocalValue(next); onValueChange?.(next); };
  return <div className="min-w-0">
    <div role="tablist" aria-label={label} className="flex gap-1 overflow-x-auto border-b border-border">
      {items.map((item, index) => <button key={item.value} type="button" role="tab"
        id={`${id}-tab-${item.value}`} aria-controls={`${id}-panel-${item.value}`}
        aria-selected={selected === item.value} tabIndex={selected === item.value ? 0 : -1}
        className={cn('shrink-0 whitespace-nowrap border-b-2 px-3 py-2 text-sm font-medium outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary', selected === item.value ? 'border-primary text-primary' : 'border-transparent text-text-secondary hover:bg-neutral-background hover:text-text-primary')}
        onClick={() => select(item.value)} onKeyDown={event => {
          let next: number;
          if (event.key === 'ArrowRight') next = (index + 1) % items.length;
          else if (event.key === 'ArrowLeft') next = (index + items.length - 1) % items.length;
          else if (event.key === 'Home') next = 0;
          else if (event.key === 'End') next = items.length - 1;
          else return;
          event.preventDefault();
          select(items[next].value);
          event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[role="tab"]')[next]?.focus();
        }}>{item.label}</button>)}
    </div>
    {items.map(item => <div key={item.value} role="tabpanel" id={`${id}-panel-${item.value}`}
      aria-labelledby={`${id}-tab-${item.value}`} hidden={selected !== item.value} tabIndex={0}
      className="pt-4 outline-none focus-visible:ring-2 focus-visible:ring-primary">{item.content}</div>)}
  </div>;
}
