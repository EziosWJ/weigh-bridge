import { useId, useState, type Ref } from 'react';
import { Button } from '@/components/ui/button';
import { normalizePlate, plateError, provinces } from '@/lib/weighing/plate';
import './plate-input.css';

export function PlateInput({ value, onChange, id, inputRef, disabled, legacyValue }: {
  value: string; onChange: (value: string) => void; id?: string; inputRef?: Ref<HTMLInputElement>; disabled?: boolean; legacyValue?: string;
}) {
  const generated = useId();
  const controlId = id ?? generated;
  const [open, setOpen] = useState(false);
  const [touched, setTouched] = useState(false);
  const province = provinces.includes(value[0]) ? value[0] : '';
  const tail = province ? value.slice(1) : value;
  const legacy = !!legacyValue && value === legacyValue && !!plateError(value);
  const error = touched && !legacy ? plateError(value) : '';
  return <div className="plate-control" onBlur={event => { if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false); }} onKeyDown={event => { if (event.key === 'Escape' && open) { event.stopPropagation(); setOpen(false); document.getElementById(`${controlId}-province`)?.focus(); } }}>
    <div className="plate-entry">
      <Button id={`${controlId}-province`} aria-label="选择车牌省份" aria-expanded={open} aria-controls={`${controlId}-provinces`} disabled={disabled} onClick={() => setOpen(!open)}>{province || '省份'}</Button>
      <input ref={inputRef} id={controlId} aria-label="车牌号码" className="plate-number" autoComplete="off" spellCheck={false} value={tail} disabled={disabled} placeholder="A12345" aria-invalid={!!error} aria-describedby={`${controlId}-hint`} onBlur={() => setTouched(true)} onChange={event => {
        const next = normalizePlate(event.target.value);
        onChange(provinces.includes(next[0]) ? next : province + next);
      }} onPaste={event => {
        const pasted = normalizePlate(event.clipboardData.getData('text'));
        if (provinces.includes(pasted[0])) { event.preventDefault(); onChange(pasted); }
      }} />
    </div>
    {open && <div id={`${controlId}-provinces`} role="group" aria-label="车牌省份" className="plate-provinces">{provinces.map(item => <Button key={item} size="sm" aria-pressed={province === item} disabled={disabled} onClick={() => { onChange(item + tail); setOpen(false); document.getElementById(controlId)?.focus(); }}>{item}</Button>)}</div>}
    <p id={`${controlId}-hint`} className={`mt-1 text-xs ${error ? 'text-error' : 'text-text-tertiary'}`}>{error || (legacy ? '' : '')}</p>
  </div>;
}
