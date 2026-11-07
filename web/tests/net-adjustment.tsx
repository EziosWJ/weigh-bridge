import { useState } from 'react';
import { createRoot } from 'react-dom/client';
import { NetAdjustmentFields } from '../src/pages/weighing/deduction-fields';
import type { NetAdjustmentsInput } from '../src/types/weighing';

export function Fixture() {
  const [value, setValue] = useState<NetAdjustmentsInput>([]);
  return <><button onClick={() => setValue([{ mode: 'percent', amount: 0, rateBp: -500, category: 'WATER', remark: '' }])}>加载历史上调</button><NetAdjustmentFields basis={{ gross: 30, tare: 12.66, net: 17.34 }} value={value} onChange={setValue} /><output>{JSON.stringify(value)}</output></>;
}
createRoot(document.getElementById('root')!).render(<Fixture />);
