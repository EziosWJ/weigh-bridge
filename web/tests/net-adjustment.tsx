import { useState } from 'react';
import { createRoot } from 'react-dom/client';
import { NetAdjustmentFields } from '../src/pages/weighing/deduction-fields';
import type { NetAdjustmentInput } from '../src/types/weighing';

export function Fixture() {
  const [value, setValue] = useState<NetAdjustmentInput>({ mode: 'percent', amount: 0, rateBp: 500, category: 'QUALITY', remark: '' });
  const [allowUpward, setAllowUpward] = useState(false);
  return <><button onClick={() => setAllowUpward(!allowUpward)}>切换更正重开</button><NetAdjustmentFields basis={{ gross: 30, tare: 12.66, net: 17.34 }} value={value} onChange={setValue} allowUpward={allowUpward} /><output>{JSON.stringify(value)}</output></>;
}
createRoot(document.getElementById('root')!).render(<Fixture />);
