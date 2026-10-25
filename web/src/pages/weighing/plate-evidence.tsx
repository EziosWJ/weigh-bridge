import type { PlateEvidence } from '@/types/weighing';

export function PlateCloseup({ plate }: { plate: string }) {
  return <div className="flex flex-wrap items-center gap-3 rounded-control border border-border bg-neutral-background p-3" aria-label="车牌参考">
    <strong className="rounded-control border-2 border-primary bg-surface px-3 py-1 text-xl font-semibold tracking-widest text-primary">{plate}</strong>
    <span className="text-xs text-text-secondary">车牌参考<br />请核对现场车辆</span>
  </div>;
}

export function PlateEvidenceInfo({ evidence }: { evidence?: PlateEvidence }) {
  return evidence ? <div className="space-y-1 text-sm" aria-label="当次车牌核对依据">
    {evidence.scenePlate && <PlateCloseup plate={evidence.scenePlate} />}
    <p>原始识别候选：{evidence.candidate || '无（人工录入／识别失败）'}</p>
    <p>人工确认车牌：{evidence.confirmedPlate}</p>
    <p>录入来源：{evidence.source === 'simulated' ? '历史模拟识别后人工核对' : evidence.source === 'agent' ? 'Agent 识别后人工核对' : evidence.source === 'server' ? 'Server 摄像头识别后人工核对' : '人工录入'}</p>
  </div> : <p className="text-sm text-text-tertiary">历史车牌识别来源：未记录</p>;
}
