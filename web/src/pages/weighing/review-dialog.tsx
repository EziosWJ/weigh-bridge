import { useEffect, useRef, useState } from 'react';
import type { FinalWeightInput, SimulatedCapture, WeightValues } from '@/types/weighing';
import { adjustedWeights, formatWeight, weightDeduction } from '@/lib/weighing/weights';
import { WeightDeductionFields } from './deduction-fields';
import { CapturePhoto } from './camera-panel';
import { X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { DetailItem } from '@/components/common/detail-item';
import { StatusTag } from '@/components/common/status-tag';
import { Dialog, DialogBody, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogOverlay, DialogTitle } from '@/components/ui/dialog';

export interface WeighingReviewValue {
  plate: string;
  direction: string;
  material: string;
  partyLabel: string;
  party: string;
  weight: number;
  kind: string;
  firstWeight?: string;
  original?: WeightValues;
  capture?: SimulatedCapture;
  supplementary: { label: string; value: string }[];
}
const steps = ['核对车牌', '核对业务信息', '核对重量', '核对模拟证据'];

/** 一次采集的一次核对会话。父页面在数据变化时卸载，进度不进入业务缓存。 */
export function WeighingReviewDialog({ value, title, onCancel, onMismatch, onConfirm }: {
  value: WeighingReviewValue;
  title: string;
  onCancel: () => void;
  onMismatch: (step: number) => void;
  onConfirm: (finalWeights: FinalWeightInput) => void;
}) {
  const [step, setStep] = useState(0);
  const [imageReady, setImageReady] = useState(false);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [finalInput, setFinalInput] = useState<FinalWeightInput>(() => ({ gross: value.original?.gross ?? Number.NaN, tare: value.original?.tare ?? Number.NaN }));
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => { heading.current?.focus(); }, [step]);
  const summary = step === steps.length;
  let validDeduction = true;
  let calculatedWeights: WeightValues | undefined;
  if (value.original) { try { calculatedWeights = adjustedWeights(value.original, finalInput); } catch { validDeduction = false; } }
  const calculatedDeduction = value.original && calculatedWeights ? weightDeduction(value.original, calculatedWeights) : undefined;
  const weight = formatWeight(value.weight);
  return <Dialog open onOpenChange={open => { if (!open) onCancel(); }} trapFocus={!previewOpen} closeOnEscape={!previewOpen} restoreFocus lockScroll closeOnOverlayClick={false}>
    <DialogOverlay />
    <DialogContent className="flex max-h-[calc(100dvh-32px)] max-w-[640px] flex-col">
      <DialogHeader className="shrink-0"><div><DialogTitle>逐项核对{title}</DialogTitle><DialogDescription>请与本次车辆及称重情况逐项比对，核对过程中不会保存称重结果。</DialogDescription></div><DialogClose aria-label="关闭核对"><X className="h-4 w-4" aria-hidden /></DialogClose></DialogHeader>
      <DialogBody className="space-y-4" aria-live="polite" aria-atomic="false">
        <div className="flex items-center justify-between gap-3 text-sm"><span className="font-medium text-text-secondary">{summary ? '4 项已核对' : `第 ${step + 1} / 4 项`}</span><StatusTag tone="warning">测试数据</StatusTag></div>
        <h3 ref={heading} className="rounded-control text-lg font-semibold focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary" tabIndex={-1}>{summary ? '确认本次称重' : steps[step]}</h3>
        {!summary && <div className="flex gap-2" aria-label="核对进度">{steps.map((label, index) => <span key={label} title={label} aria-current={index === step ? 'step' : undefined} className={`h-1 flex-1 rounded ${index <= step ? 'bg-primary' : 'bg-border'}`} />)}</div>}
        {step === 0 && <><p className="text-sm text-text-secondary">请看当前车辆实际车牌，确认与下方完全一致。</p><p className="rounded-control border border-border bg-neutral-background p-5 text-center text-2xl font-semibold tracking-wide">{value.plate}</p><p className="text-sm text-text-tertiary">车牌为手工填写，未接入自动识别。</p></>}
        {step === 1 && <><p className="text-sm text-text-secondary">确认这是同一趟运输，方向、物料和供收方均正确。</p><div className="grid gap-4 rounded-control border border-border p-4 sm:grid-cols-2"><DetailItem label="运输方向" value={value.direction} /><DetailItem label="物料及规格／编码" value={value.material} /><DetailItem label={value.partyLabel} value={value.party} /></div>{value.supplementary.length > 0 && <details><summary className="cursor-pointer rounded-control text-sm text-text-secondary focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary">查看补充运输信息</summary><div className="mt-3 grid gap-3 sm:grid-cols-2">{value.supplementary.map(item => <DetailItem key={item.label} label={item.label} value={item.value} />)}</div></details>}</>}
        {step === 2 && <><p className="text-sm text-text-secondary">请核对本次重量及毛重／皮重含义，确保车辆处于本次应称的装载状态。</p><div className="rounded-control border border-border bg-neutral-background p-5 text-center"><p className="text-sm text-text-secondary">本次{value.kind} · 模拟稳定重量</p><p className="mt-2 font-mono text-3xl font-semibold tabular-nums">{weight}</p></div>{value.firstWeight && <p className="text-sm text-text-secondary">首次依据：{value.firstWeight}</p>}</>}
        {step === 3 && <><StatusTag tone="info">本次模拟抓拍</StatusTag><p className="text-sm text-text-secondary">请查看图片及关联信息：车牌 <strong>{value.plate}</strong>，本次{value.kind} <strong>{weight}</strong>。</p>{value.capture ? <CapturePhoto capture={value.capture} onReady={setImageReady} onPreviewChange={setPreviewOpen} /> : <p role="alert" className="text-error">缺少本次模拟图片，请返回重新采集。</p>}<p className="text-xs text-text-tertiary">内置示例图片，非当前车辆现场拍摄；关联车牌为手填，摄像头未接入。</p></>}
        {summary && <><p className="text-sm text-text-secondary">以下四项均已逐项核对。确认后将保存本次测试称重。</p><div className="grid gap-4 rounded-control border border-border p-4 sm:grid-cols-2"><DetailItem label="车牌" value={value.plate} /><DetailItem label="运输方向" value={value.direction} /><DetailItem label="物料及规格／编码" value={value.material} /><DetailItem label={value.partyLabel} value={value.party} /><DetailItem label={`本次${value.kind}`} value={<strong>{weight}</strong>} /><DetailItem label="证据状态" value="模拟抓拍已核对，摄像头未接入" /></div><p className="text-sm text-text-tertiary">仍有疑问请返回核对或关闭，关闭不会提交。</p></>}
        {summary && value.original && <WeightDeductionFields original={value.original} value={finalInput} onChange={setFinalInput} />}
      </DialogBody>
      <DialogFooter className="shrink-0 flex-wrap items-center justify-between">
        {summary && calculatedWeights && calculatedDeduction && <p className="w-full text-xs tabular-nums text-text-secondary" aria-live="polite">系统计算扣减：毛重 {formatWeight(calculatedDeduction.grossDeduction)} · 皮重 {formatWeight(calculatedDeduction.tareDeduction)} · 最终净重 <strong className="text-text-primary">{formatWeight(calculatedWeights.net)}</strong></p>}
        <div className="flex gap-2">{step > 0 && <Button size="lg" onClick={() => setStep(step - 1)}>上一项</Button>}<Button size="lg" variant="ghost" onClick={() => onMismatch(step)}>不一致，返回修改</Button></div>
        {summary ? <Button key="submit-weighing" size="lg" variant="primary" disabled={!validDeduction} onClick={() => onConfirm(finalInput)}>{value.original ? '确认称重并出单' : `确认${title}`}</Button> : <Button key={`review-step-${step}`} size="lg" variant="primary" disabled={step === 3 && !imageReady} onClick={() => setStep(step + 1)}>{step === 3 ? '已核对，查看汇总' : '一致，下一项'}</Button>}
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
