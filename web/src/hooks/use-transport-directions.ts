import { useEffect, useState } from 'react';
import { getDictItems } from '@/api/system';
import { directionLabels, type Direction } from '@/types/weighing';

/** 字典只改变显示名称；业务编码仍由系统固定。 */
export function useTransportDirections() {
  const [labels, setLabels] = useState<Record<Direction, string>>(directionLabels);
  useEffect(() => {
    let active = true;
    void getDictItems('TRANSPORT_DIRECTION').then(items => {
      if (!active) return;
      const next = { ...directionLabels };
      for (const item of items) {
        const value = item.value.trim().toLowerCase();
        if ((value === 'inbound' || value === 'outbound') && item.label.trim()) next[value] = item.label;
      }
      setLabels(next);
    }).catch(() => { if (active) setLabels(directionLabels); });
    return () => { active = false; };
  }, []);
  return labels;
}
