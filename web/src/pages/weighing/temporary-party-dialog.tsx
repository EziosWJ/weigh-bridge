import { useId, useState } from 'react';
import { weighingApi } from '@/api/weighing';
import { Field } from '@/components/common/field';
import { FormDialog } from '@/components/common/form-dialog';
import { StatusTag } from '@/components/common/status-tag';
import { toast } from '@/components/common/toast-store';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { getErrorMessage } from '@/lib/api-error';
import { useAuthStore } from '@/store/auth-store';
import { useWeighingStore } from '@/store/weighing-store';
import { partyNatureLabels, partyRoleLabels, type Party, type PartyKind, type PartyNature } from '@/types/weighing';
import { isAvailableParty } from './business-options';

export function TemporaryPartyDialog({ current, initialName, label, role, loadParties, onSelect, onCancel }: {
  current: Party | null;
  initialName: string;
  label: string;
  role: PartyKind;
  loadParties: () => Promise<Party[]>;
  onSelect: (party: Party) => void;
  onCancel: () => void;
}) {
  const id = useId();
  const [name, setName] = useState(current?.name ?? initialName);
  const [nature, setNature] = useState<PartyNature | ''>(current?.nature ?? '');
  const [roles, setRoles] = useState<PartyKind[]>(current?.roles ?? [role]);
  const [matches, setMatches] = useState<Party[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const editableRoles = (Object.keys(partyRoleLabels) as PartyKind[]).filter(kind => current || kind !== role);
  const available = (party: Party) => isAvailableParty(party) && party.roles.includes(role);

  async function save(existing?: Party) {
    const cleanName = name.trim();
    if (saving || (!existing && (!cleanName || !nature || roles.length === 0))) return;
    if (!existing && Array.from(cleanName).length > 100) {
      setError('名称最多填写 100 个字符。');
      return;
    }

    setSaving(true);
    setError('');
    const token = useAuthStore.getState().token;
    const vehicle = useWeighingStore.getState().data;
    let checking = !current && (Boolean(existing) || matches.length === 0);
    try {
      let selected: Party | undefined;
      if (checking) {
        const candidates = (await loadParties()).filter(available);
        if (existing) {
          selected = candidates.find(party => party.id === existing.id && party.name.trim() === cleanName);
          if (!selected) throw new Error('该档案已不可选，请重新核对。');
        } else {
          const sameName = candidates.filter(party => party.name.trim() === cleanName);
          if (sameName.length > 0) {
            setMatches(sameName);
            return;
          }
        }
      }
      if (!selected && nature) {
        checking = false;
        selected = await weighingApi.saveTemporaryParty({ name: cleanName, nature, roles, version: current?.version }, current?.id);
        if (token !== useAuthStore.getState().token) return;
        const saved = selected;
        useWeighingStore.setState(({ data }) => ({
          data: { ...data, parties: data.parties.some(item => item.id === saved.id)
            ? data.parties.map(item => item.id === saved.id ? saved : item)
            : [...data.parties, saved] },
        }));
      }
      if (!selected || token !== useAuthStore.getState().token) return;

      const draft = useWeighingStore.getState().data;
      if (draft.draftRequestId === vehicle.draftRequestId && draft.draft.plate === vehicle.draft.plate &&
          draft.draft.direction === vehicle.draft.direction && !draft.pending) onSelect(selected);
      if (!existing) toast.success(current ? '临时资料已更正' : `临时${label}已登记`);
      onCancel();
    } catch (failure) {
      const fallback = checking ? '无法核对本厂档案，请重试' : '保存临时资料失败';
      setError(getErrorMessage(failure, fallback));
    } finally {
      setSaving(false);
    }
  }

  return <FormDialog open title={current ? `更正临时${label}` : `登记临时${label}`} description={current?.code}
    contentClassName="workbench-modal workbench-temporary-party-dialog max-w-[500px]" loading={saving} loadingText="正在处理…"
    submitText={current ? '保存更正' : matches.length ? '仍然登记并选用' : '保存并选用'}
    submitDisabled={!name.trim() || !nature || roles.length === 0} onSubmit={() => save()} onCancel={onCancel}>
    <fieldset disabled={saving} className="space-y-3">
      <Field label="名称" htmlFor={`${id}-name`} required>
        <Input id={`${id}-name`} autoFocus required value={name} onChange={event => {
          setName(event.target.value);
          setMatches([]);
          setError('');
        }} />
      </Field>
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">主体性质<span className="ml-1 text-error">*</span></legend>
        <div className="grid grid-cols-2 gap-2">
          {(Object.keys(partyNatureLabels) as PartyNature[]).map(kind =>
            <label key={kind} className={`workbench-party-nature${nature === kind ? ' is-selected' : ''}`}>
              <input type="radio" name={`${id}-nature`} value={kind} required checked={nature === kind} onChange={() => setNature(kind)} />
              {partyNatureLabels[kind]}
            </label>
          )}
        </div>
      </fieldset>
      {!current && <p className="text-sm text-text-secondary">本次用作 <strong className="text-text-primary">{label}</strong></p>}
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">{current ? '业务身份' : '同时用于（选填）'}</legend>
        <div className="flex flex-wrap gap-x-4 gap-y-2 text-sm">
          {editableRoles.map(kind => <label key={kind} className="inline-flex min-h-8 items-center gap-2">
            <input type="checkbox" checked={roles.includes(kind)} onChange={event => setRoles(previous =>
              event.target.checked ? [...previous, kind] : previous.filter(value => value !== kind)
            )} />{partyRoleLabels[kind]}
          </label>)}
        </div>
      </fieldset>
      {matches.length > 0 && <section className="space-y-2 rounded-control border border-warning-border bg-warning-background p-3" aria-label="同名档案核对">
        <h3 role="status" className="text-sm font-medium">本厂已有同名{label}</h3>
        <ul className="max-h-40 space-y-2 overflow-auto">
          {matches.map(party => <li key={party.id} className="flex flex-wrap items-center justify-between gap-2 text-sm">
            <div className="min-w-0">
              <p className="break-words font-medium">{party.name}</p>
              <p className="flex flex-wrap items-center gap-2 text-text-secondary">
                <span className="break-all">{party.code}</span><StatusTag>{party.temporaryState ? '临时' : '正式'}</StatusTag>
                <span>{partyNatureLabels[party.nature]}</span>
              </p>
            </div>
            <Button size="sm" aria-label={`选用已有档案 ${party.code}`} onClick={() => void save(party)}>选用此档案</Button>
          </li>)}
        </ul>
      </section>}
      {error && <p role="alert" className="text-sm text-error">{error}</p>}
    </fieldset>
  </FormDialog>;
}
