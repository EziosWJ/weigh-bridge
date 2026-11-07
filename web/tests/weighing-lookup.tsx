import { useState } from 'react';
import { createRoot } from 'react-dom/client';
import { BusinessRequiredFields, BusinessSupplementaryFields } from '@/pages/weighing/shared';
import { emptyTaskInput } from '@/types/weighing';
import { useWeighingStore } from '@/store/weighing-store';
import '@/styles/globals.css';

export function Fixture() {
  const [value, setValue] = useState(emptyTaskInput());
  const data = useWeighingStore(s => s.data);
  return <><BusinessRequiredFields value={value} update={patch => setValue(current => ({ ...current, ...patch }))} compact /><BusinessSupplementaryFields value={value} update={patch => setValue(current => ({ ...current, ...patch }))} />
    <output aria-label="当前选择">{JSON.stringify({ value, material: data.materials.find(m => m.id === value.materialId)?.name, party: data.parties.find(p => p.id === value.partyId)?.name, carrier: data.parties.find(p => p.id === value.carrierId)?.name })}</output></>;
}
createRoot(document.getElementById('root')!).render(<Fixture />);
