import type { PlateEvidence } from '@/types/weighing';

export function PlateCloseup({ plate }: { plate: string }) {
  return <div className="flex flex-wrap items-center gap-3 rounded-control border border-border bg-neutral-background p-3" aria-label="模拟现场车牌特写">
    <strong className="rounded-control border-2 border-primary bg-surface px-3 py-1 text-xl font-semibold tracking-widest text-primary">{plate}</strong>
    <span className="text-xs text-text-secondary">模拟车牌特写<br />与固定车辆示例组合，非照片原车牌</span>
  </div>;
}

export function PlateEvidenceInfo({ evidence }: { evidence?: PlateEvidence }) {
  return evidence ? <div className="space-y-1 text-sm" aria-label="当次车牌核对依据">
    {evidence.scenePlate && <PlateCloseup plate={evidence.scenePlate} />}
    <p>原始模拟候选：{evidence.candidate || '无（人工录入／识别失败）'}</p>
    <p>人工确认车牌：{evidence.confirmedPlate}</p>
    <p>录入来源：{evidence.source === 'simulated' ? '模拟识别后人工核对' : '人工录入'}</p>
  </div> : <p className="text-sm text-text-tertiary">历史车牌识别来源：未记录</p>;
}
