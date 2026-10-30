import { useId, useState, type Ref } from 'react';
import { X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { limitPlate, normalizePlate, plateError, provinces, PLATE_MAX_LENGTH } from '@/lib/weighing/plate';
import './plate-input.css';

export function PlateInput({ value, onChange, id, inputRef, disabled, legacyValue, clearable = false }: {
  value: string; onChange: (value: string) => void; id?: string; inputRef?: Ref<HTMLInputElement>; disabled?: boolean; legacyValue?: string; clearable?: boolean;
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
      <div className="plate-number-wrap"><input ref={inputRef} id={controlId} aria-label="车牌号码" className={`plate-number${clearable ? ' is-clearable' : ''}`} autoComplete="off" spellCheck={false} value={tail} maxLength={PLATE_MAX_LENGTH} disabled={disabled} placeholder="请输入车牌号码" aria-invalid={!!error} aria-describedby={`${controlId}-hint`} onBlur={() => setTouched(true)} onChange={event => {
        onChange(limitPlate(event.target.value, province));
      }} onPaste={event => {
        const pasted = normalizePlate(event.clipboardData.getData('text'));
        if (provinces.includes(pasted[0])) { event.preventDefault(); onChange(limitPlate(pasted, province)); }
      }} />{clearable && value && <button type="button" className="plate-clear" aria-label="清除车牌" title="清除车牌" disabled={disabled} onClick={() => { setTouched(false); setOpen(false); onChange(''); document.getElementById(controlId)?.focus(); }}><X className="h-4 w-4" aria-hidden /></button>}</div>
    </div>
    {open && <div id={`${controlId}-provinces`} role="group" aria-label="车牌省份" className="plate-provinces">{provinces.map(item => <Button key={item} size="sm" aria-pressed={province === item} disabled={disabled} onClick={() => { onChange(limitPlate(item + tail, item)); setOpen(false); document.getElementById(controlId)?.focus(); }}>{item}</Button>)}</div>}
    <p id={`${controlId}-hint`} className={`mt-1 text-xs ${error ? 'text-error' : 'text-text-tertiary'}`}>{error || (legacy ? '' : '')}</p>
  </div>;
}
