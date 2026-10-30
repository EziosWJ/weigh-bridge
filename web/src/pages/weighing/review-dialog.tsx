import { useEffect, useRef, useState, type ReactNode } from 'react';
import {
  ArrowLeft,
  ArrowRight,
  Camera,
  Check,
  ClipboardCheck,
  FileCheck2,
  Info,
  Package,
  Scale,
  Truck,
  X,
  TriangleAlert,
} from 'lucide-react';
import type { CaptureSnapshot, NetAdjustmentInput, WeightValues } from '@/types/weighing';
import { formatWeight, formatWeightValue, resolveNetAdjustment } from '@/lib/weighing/weights';
import { Button } from '@/components/ui/button';
import { StatusTag } from '@/components/common/status-tag';
import { HelpHint } from '@/components/common/help-hint';
import { Dialog, DialogBody, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogOverlay, DialogTitle } from '@/components/ui/dialog';
import { NetAdjustmentFields } from './deduction-fields';
import { CapturePhoto } from './camera-panel';
import { cn } from '@/lib/utils';

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
  expected?: WeightValues;
  capture?: CaptureSnapshot;
  supplementary: { label: string; value: string }[];
}

function ReviewFact({ label, value, wide = false, emphasis = false }: {
  label: string;
  value?: ReactNode;
  wide?: boolean;
  emphasis?: boolean;
}) {
  const empty = value === undefined || value === null || value === '';
  return <div className={cn('review-fact', wide && 'review-fact--wide', emphasis && 'review-fact--emphasis')}>
    <div className="min-w-0">
      <p className="review-fact-label">{label}</p>
      <p className="review-fact-value">{empty ? '—' : value}</p>
    </div>
  </div>;
}

function ReviewSection({ icon, title, extra, children }: {
  icon: ReactNode;
  title: string;
  extra?: ReactNode;
  children: ReactNode;
}) {
  return <section className="review-section">
    <header className="review-section-header">
      <div className="review-section-title"><span aria-hidden>{icon}</span><h3>{title}</h3></div>
      {extra}
    </header>
    <div className="review-section-body">{children}</div>
  </section>;
}

/** 集中核对保留本次快照；失效后保留内容供查看，同时禁止提交。 */
export function WeighingReviewDialog({ value, title, missing = [], businessEditor, invalidReason = '', submitting = false, submitError = '', onCancel, onMismatch, onConfirm }: {
  value: WeighingReviewValue;
  title: string;
  missing?: string[];
  businessEditor?: ReactNode;
  invalidReason?: string;
  submitting?: boolean;
  submitError?: string;
  onCancel: () => void;
  onRecapture?: () => void;
  onMismatch: () => void;
  onConfirm: (netAdjustment?: NetAdjustmentInput) => void;
}) {
  const [imageReady, setImageReady] = useState(false);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [step, setStep] = useState<'review' | 'final'>('review');
  const [netAdjustment, setNetAdjustment] = useState<NetAdjustmentInput>({ mode: 'amount', amount: 0, rateBp: 0, category: '', remark: '' });
  const heading = useRef<HTMLHeadingElement>(null);
  const finalHeading = useRef<HTMLHeadingElement>(null);
  const second = !!value.original;
  const showFinal = second && step === 'final';
  useEffect(() => { heading.current?.focus(); }, []);
  useEffect(() => { if (showFinal) finalHeading.current?.focus(); }, [showFinal]);
  let adjustmentError = '';
  let resolvedAdjustment = 0;
  if (value.original) {
    try {
      resolvedAdjustment = resolveNetAdjustment(value.original, netAdjustment).amount;
    } catch (error) {
      adjustmentError = error instanceof Error ? error.message : '请核对净重调整';
    }
  }
  const requiresImage = !!value.capture && value.capture.captureRequired !== false && value.capture.imageStatus !== 'disabled';
  const reviewReady = !!value.capture && (!requiresImage || imageReady) && !invalidReason && missing.length === 0;
  const finalReady = reviewReady && !adjustmentError;
  const imageCount = value.capture ? value.capture.images?.length || (value.capture.imageSrc ? 1 : 0) : 0;
  const capturePolicy = value.capture?.captureRequired === false ? '非强制抓拍' : '强制抓拍';

  return <Dialog open onOpenChange={open => { if (!open && !submitting) onCancel(); }} trapFocus={!previewOpen} closeOnEscape={!previewOpen && !submitting} restoreFocus lockScroll closeOnOverlayClick={false}>
    <DialogOverlay />
    <DialogContent className="weighing-review flex max-h-[calc(100dvh-32px)] max-w-[1180px] flex-col">
      <DialogHeader className="weighing-review-header">
        <div className="flex min-w-0 items-center gap-3">
          <span className="review-header-icon" aria-hidden><ClipboardCheck className="h-5 w-5" /></span>
          <div className="min-w-0">
            <DialogTitle className="sr-only">核对{title}</DialogTitle>
            <div className="review-eyebrow">
              <span className="review-eyebrow-title">{second ? '回磅核对' : '首磅核对'} · {value.kind}</span>
              {invalidReason && <span className="review-eyebrow-status is-invalid">核对已失效</span>}
            </div>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <StatusTag tone="warning">人工核对</StatusTag>
          <DialogClose aria-label="关闭核对" disabled={submitting}><X className="h-4 w-4" aria-hidden /></DialogClose>
        </div>
      </DialogHeader>

      <DialogBody className="weighing-review-body">
        {invalidReason && <div className="review-alert" role="alert">
          <TriangleAlert className="h-5 w-5" aria-hidden />
          <div><strong>当前核对内容已失效</strong><p>{invalidReason} 当前显示原核对内容，不能提交；请关闭窗口后重新采集。</p></div>
        </div>}
        {!invalidReason && missing.length > 0 && <div className="review-missing" role="status">
          <TriangleAlert className="h-5 w-5" aria-hidden />
          <div><strong>还缺{missing.join('、')}</strong><p>重量与抓拍已冻结；补齐以下资料后可直接确认，无需重新采集。</p></div>
        </div>}
        {!invalidReason && missing.length > 0 && businessEditor && <section className="review-business-fill" aria-label="补齐本趟资料">{businessEditor}</section>}

        {showFinal ? <section className="review-compact-summary" aria-label="本次称重摘要">
          <strong>{value.plate || '待核对'}</strong><span>{value.direction}</span><span>回磅 · {value.kind}</span><span>本次采集 <b>{formatWeight(value.weight)}</b></span>
        </section> : <section className="review-summary" aria-label="本次称重摘要">
          <div className="review-hero review-hero--vehicle">
            <p className="review-hero-label"><Truck className="h-4 w-4" aria-hidden />当前车辆</p>
            <h3 ref={heading} tabIndex={-1} className="review-plate">{value.plate || '待核对'}</h3>
          </div>
          <div className="review-hero review-hero--weight">
            <p className="review-hero-label"><Scale className="h-4 w-4" aria-hidden />本次采集{value.kind}</p>
            <div className="review-weight">{formatWeightValue(value.weight)}<span>吨</span></div>
          </div>
        </section>}

        {showFinal ? <section className="review-final-step" aria-label="净重调整">
          <div className="review-final-step-head">
            <span className="review-step-badge"><Info className="h-3.5 w-3.5" aria-hidden />第 2 步 / 共 2 步</span>
            <div className="review-final-title"><h3 ref={finalHeading} tabIndex={-1}>最终重量确认</h3><HelpHint label="最终重量确认说明">毛重、皮重沿用设备原始值；请核对原始重量并录入净重扣减。确认后将生成正式单据，原始采集依据保留。</HelpHint></div>
          </div>
          <ReviewSection icon={<Scale className="h-4 w-4" aria-hidden />} title="净重调整" extra={<StatusTag tone="info">可扣减</StatusTag>}>
            <NetAdjustmentFields basis={value.original!} value={netAdjustment} onChange={setNetAdjustment} showError={false} />
          </ReviewSection>
        </section> : <section className="review-main" aria-label="本趟运输核对依据">
          <ReviewSection icon={<Package className="h-4 w-4" aria-hidden />} title="业务核对" extra={<HelpHint label="业务核对说明" align="right">请与现场车辆核对。</HelpHint>}>
            <div className="review-facts">
              <ReviewFact label="运输方向" value={value.direction} />
              <ReviewFact label="物料及规格／编码" value={value.material} />
              <ReviewFact label={value.partyLabel} value={value.party} />
              {value.firstWeight && <ReviewFact label="首次称重依据" value={value.firstWeight} emphasis />}
              {value.expected && <ReviewFact label="预计净重（未出单）" value={formatWeight(value.expected.net)} emphasis />}
              {value.supplementary.map((item, index) => <ReviewFact key={`${item.label}-${index}`} label={item.label} value={item.value} wide={item.label === '备注'} />)}
            </div>
          </ReviewSection>

          <ReviewSection
            icon={<Camera className="h-4 w-4" aria-hidden />}
            title="本次抓拍"
            extra={value.capture ? <span className="review-section-extra">{imageCount} 张 · {capturePolicy}</span> : undefined}
          >
            {value.capture ? <CapturePhoto gallery capture={value.capture} onReady={setImageReady} onPreviewChange={setPreviewOpen} /> : <div className="review-empty" role="alert">
              <Camera className="h-5 w-5" aria-hidden />
              <div><strong>缺少本次采集依据</strong><p>请返回重新采集称重与抓拍后再确认。</p></div>
            </div>}
            {!requiresImage && value.capture && <p className="review-hint">本次未强制抓拍，允许无图确认；请对照现场车辆核对。</p>}
          </ReviewSection>
        </section>}
      </DialogBody>

      <DialogFooter className="weighing-review-footer">
        <div className="review-footer-status">
          {submitting && <p role="status" className="review-footer-info">正在保存称重，请勿关闭窗口…</p>}
          {!submitting && missing.length > 0 && <p className="review-footer-info">补齐{missing.join('、')}后可直接确认，重量与图片无需重新采集。</p>}
          {!submitting && requiresImage && !imageReady && <p className="review-footer-info">正在加载本次抓拍图片…</p>}
          {invalidReason && <p className="review-footer-alert">核对已失效，请返回重新采集。</p>}
          {showFinal && adjustmentError && <p role="alert" className="review-footer-error">无法出单：{adjustmentError}</p>}
          {submitError && <p className="review-footer-error">保存失败：{submitError}</p>}
          {!showFinal && <div className="review-confirm-note"><FileCheck2 className="h-4 w-4" aria-hidden /><span>请确认车辆已完全上磅，现场与资料一致。</span><HelpHint label="称重确认说明" above>{second ? '下一步核对原始重量并录入净重调整；确认后才生成正式单据。' : `确认后保存首磅（${value.kind}）并建立运输任务。`}</HelpHint></div>}
        </div>
        <div className="review-actions">
          {showFinal ? <>
            <Button size="lg" disabled={submitting} onClick={() => setStep('review')}><ArrowLeft className="h-4 w-4" aria-hidden />上一步</Button>
            <Button size="lg" variant="primary" className="disabled:opacity-50" disabled={!finalReady || submitting} aria-busy={submitting} onClick={() => { if (finalReady && !submitting) onConfirm(resolvedAdjustment === 0 ? undefined : netAdjustment); }}>
              <Check className="h-4 w-4" aria-hidden />{submitting ? '正在保存…' : '确认回磅并出单'}
            </Button>
          </> : <>
            <Button size="lg" disabled={submitting} onClick={invalidReason ? onCancel : onMismatch}><ArrowLeft className="h-4 w-4" aria-hidden />{invalidReason ? '返回重新采集' : '不一致，返回修改'}</Button>
            {second ? <Button size="lg" variant="primary" className="disabled:opacity-50" disabled={!reviewReady || submitting} onClick={() => { if (reviewReady && !submitting) setStep('final'); }}>
              <ArrowRight className="h-4 w-4" aria-hidden />下一步：核对最终重量
            </Button> : <Button size="lg" variant="primary" className="disabled:opacity-50" disabled={!reviewReady || submitting} onClick={() => { if (reviewReady && !submitting) onConfirm(); }}>
              <Check className="h-4 w-4" aria-hidden />确认首磅（{value.kind}）
            </Button>}
          </>}
        </div>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
