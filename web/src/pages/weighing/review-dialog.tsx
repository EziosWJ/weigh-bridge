import { useEffect, useRef, useState } from 'react';
import { X } from 'lucide-react';
import type { FinalWeightInput, CaptureSnapshot, WeightValues } from '@/types/weighing';
import { adjustedWeights, formatWeight, weightDeduction } from '@/lib/weighing/weights';
import { Button } from '@/components/ui/button';
import { DetailItem } from '@/components/common/detail-item';
import { SectionTabs } from '@/components/common/section-tabs';
import { StatusTag } from '@/components/common/status-tag';
import { Dialog, DialogBody, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogOverlay, DialogTitle } from '@/components/ui/dialog';
import { WeightDeductionFields } from './deduction-fields';
import { CapturePhoto } from './camera-panel';

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
  capture?: CaptureSnapshot;
  supplementary: { label: string; value: string }[];
}

/** 集中核对保留本次快照；失效后保留内容供查看，同时禁止提交。 */
export function WeighingReviewDialog({ value, title, invalidReason = '', submitting = false, submitError = '', onCancel, onMismatch, onConfirm }: {
  value: WeighingReviewValue;
  title: string;
  invalidReason?: string;
  submitting?: boolean;
  submitError?: string;
  onCancel: () => void;
  onMismatch: () => void;
  onConfirm: (finalWeights: FinalWeightInput) => void;
}) {
  const [imageReady, setImageReady] = useState(false);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [finalInput, setFinalInput] = useState<FinalWeightInput>(() => ({ gross: value.original?.gross ?? Number.NaN, tare: value.original?.tare ?? Number.NaN }));
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => { heading.current?.focus(); }, []);
  let deductionError = '';
  let calculatedWeights: WeightValues | undefined;
  if (value.original) { try { calculatedWeights = adjustedWeights(value.original, finalInput); } catch (error) { deductionError = error instanceof Error ? error.message : '请核对最终重量'; } }
  const calculatedDeduction = value.original && calculatedWeights ? weightDeduction(value.original, calculatedWeights) : undefined;
  const ready = !!value.capture && imageReady && !deductionError && !invalidReason;

  return <Dialog open onOpenChange={open => { if (!open && !submitting) onCancel(); }} trapFocus={!previewOpen} closeOnEscape={!previewOpen && !submitting} restoreFocus lockScroll closeOnOverlayClick={false}>
    <DialogOverlay />
    <DialogContent className="weighing-review flex max-h-[calc(100dvh-32px)] max-w-[1080px] flex-col">
      <DialogHeader className="shrink-0">
        <div className="min-w-0"><DialogTitle>核对{title}</DialogTitle><DialogDescription>请对照现场车辆核对以下信息，确认后保存本次称重。</DialogDescription></div>
        <div className="flex shrink-0 items-center gap-2 whitespace-nowrap"><StatusTag tone="warning">人工核对</StatusTag><DialogClose aria-label="关闭核对" disabled={submitting}><X className="h-4 w-4" aria-hidden /></DialogClose></div>
      </DialogHeader>
      <DialogBody className="grid items-start gap-6 md:grid-cols-[minmax(0,0.85fr)_minmax(0,1.4fr)]">
        <section aria-label="本趟运输核对依据" className="min-w-0 space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border pb-4">
            <h3 ref={heading} tabIndex={-1} className="rounded-control text-2xl font-semibold focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary">{value.plate}</h3>
            <span className="text-sm text-text-secondary">{value.direction}</span>
          </div>
          <SectionTabs label="运输核对信息" items={[
            { value: 'basic', label: '基本信息', content: <div className="grid gap-3 sm:grid-cols-2 md:grid-cols-1">
            <DetailItem label="物料及规格／编码" value={value.material} />
            <DetailItem label={value.partyLabel} value={value.party} />
            {value.firstWeight && <DetailItem label="首次称重依据" value={value.firstWeight} />}
          </div> },
            ...(value.supplementary.length ? [{ value: 'supplementary', label: '补充运输信息', content: <div className="grid gap-3 sm:grid-cols-2 md:grid-cols-1">{value.supplementary.map(item => <DetailItem key={item.label} label={item.label} value={item.value} />)}</div> }] : []),
          ]} />
          <div className="space-y-2 border-t border-border pt-3">
            <h4 className="text-sm font-medium">本次抓拍</h4>
            {value.capture ? <CapturePhoto capture={value.capture} onReady={setImageReady} onPreviewChange={setPreviewOpen} /> : <p role="alert" className="text-error">缺少本次抓拍图片，请返回重新采集。</p>}
            <p className="text-xs text-text-tertiary">静态示例，非现场抓拍；摄像头未接入。</p>
          </div>

        </section>
        <section aria-label={value.original ? '最终重量与调整' : '本次称重依据'} className="min-w-0 space-y-5 md:border-l md:border-border md:pl-6">
          <div className="flex flex-wrap items-center justify-between gap-4 rounded-control bg-neutral-background p-4">
            <div><p className="text-sm text-text-secondary">本次采集{value.kind}</p><p className="mt-1 text-xs text-text-tertiary">请核对车辆完全上磅及装载状态</p></div>
            <strong className="shrink-0 text-3xl tabular-nums">{formatWeight(value.weight)}</strong>
          </div>
          {value.original ? <WeightDeductionFields original={value.original} value={finalInput} onChange={setFinalInput} showError={false} /> : <div className="space-y-3 text-sm leading-relaxed text-text-secondary"><h4 className="font-medium text-text-primary">确认本次称重</h4><p>请核对车牌、运输方向、物料、供收方及本次重量，并对照图片检查车辆位置。</p><p>确认后保存首次称重，装卸完成后按车牌办理回磅。</p></div>}
        </section>
      </DialogBody>
      <DialogFooter className="shrink-0 flex-wrap items-center justify-between">
        {invalidReason && <p role="alert" className="w-full text-sm text-warning">{invalidReason} 当前显示的是原核对内容，不能提交；关闭窗口后重新采集。</p>}
        {deductionError && <p role="alert" className="w-full text-xs text-error">无法出单：{deductionError}</p>}
        {submitError && <p role="alert" className="w-full text-sm text-error">保存失败：{submitError}</p>}
        {submitting && <p role="status" className="w-full text-sm text-text-secondary">正在保存称重，请勿关闭窗口…</p>}
        {!imageReady && <p className="w-full text-xs text-warning">请等待本次抓拍图片加载；加载失败时返回重新采集。</p>}
        {calculatedWeights && calculatedDeduction && <div className="w-full space-y-2" aria-live="polite">
          <div className="grid grid-cols-3 gap-3 rounded-control bg-neutral-background px-3 py-2 text-xs text-text-secondary">
            <div>最终毛重<strong className="mt-1 block text-base font-semibold text-text-primary">{formatWeight(calculatedWeights.gross)}</strong></div>
            <div>最终皮重<strong className="mt-1 block text-base font-semibold text-text-primary">{formatWeight(calculatedWeights.tare)}</strong></div>
            <div>最终净重<strong className="mt-1 block text-base font-semibold text-primary">{formatWeight(calculatedWeights.net)}</strong></div>
          </div>
          <p className="text-xs tabular-nums text-text-secondary">系统计算调整差额：毛重 {formatWeight(calculatedDeduction.grossDeduction)} · 皮重 {formatWeight(calculatedDeduction.tareDeduction)}</p>
        </div>}
        <Button size="lg" disabled={submitting} onClick={invalidReason ? onCancel : onMismatch}>{invalidReason ? '返回重新采集' : '不一致，返回修改'}</Button>
        <Button size="lg" variant="primary" className="disabled:opacity-50" disabled={!ready || submitting} aria-busy={submitting} onClick={() => { if (ready && !submitting) onConfirm(finalInput); }}>{submitting ? '正在保存…' : value.original ? '确认称重并出单' : `确认${title}`}</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
