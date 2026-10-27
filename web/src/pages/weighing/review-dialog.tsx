import { useEffect, useRef, useState, type ReactNode } from 'react';
import {
  ArrowLeft,
  ArrowRight,
  Building2,
  Camera,
  Check,
  CircleCheck,
  ClipboardCheck,
  FileCheck2,
  FileText,
  Info,
  Package,
  Scale,
  Truck,
  UserRound,
  X,
  TriangleAlert,
} from 'lucide-react';
import type { FinalWeightInput, CaptureSnapshot, WeightValues } from '@/types/weighing';
import { adjustedWeights, formatWeight, formatWeightValue, weightDeduction } from '@/lib/weighing/weights';
import { Button } from '@/components/ui/button';
import { StatusTag } from '@/components/common/status-tag';
import { Dialog, DialogBody, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogOverlay, DialogTitle } from '@/components/ui/dialog';
import { WeightDeductionFields } from './deduction-fields';
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
  capture?: CaptureSnapshot;
  supplementary: { label: string; value: string }[];
}

function ReviewFact({ icon, label, value, wide = false, emphasis = false }: {
  icon: ReactNode;
  label: string;
  value?: ReactNode;
  wide?: boolean;
  emphasis?: boolean;
}) {
  const empty = value === undefined || value === null || value === '';
  return <div className={cn('review-fact', wide && 'review-fact--wide', emphasis && 'review-fact--emphasis')}>
    <span className="review-fact-icon" aria-hidden>{icon}</span>
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

function ReviewSubsection({ title, extra, children }: { title: string; extra: string; children: ReactNode }) {
  return <div className="review-subsection">
    <div className="review-subsection-head"><h4>{title}</h4><span>{extra}</span></div>
    {children}
  </div>;
}

function supplementaryIcon(label: string) {
  if (label.includes('承运')) return <Truck className="h-4 w-4" aria-hidden />;
  if (label.includes('联系人')) return <UserRound className="h-4 w-4" aria-hidden />;
  if (label.includes('司机')) return <UserRound className="h-4 w-4" aria-hidden />;
  if (label.includes('备注')) return <FileText className="h-4 w-4" aria-hidden />;
  return <Info className="h-4 w-4" aria-hidden />;
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
  const [step, setStep] = useState<'review' | 'final'>('review');
  const [finalInput, setFinalInput] = useState<FinalWeightInput>(() => ({ gross: value.original?.gross ?? Number.NaN, tare: value.original?.tare ?? Number.NaN }));
  const heading = useRef<HTMLHeadingElement>(null);
  const finalHeading = useRef<HTMLHeadingElement>(null);
  const second = !!value.original;
  const showFinal = second && step === 'final';
  useEffect(() => { heading.current?.focus(); }, []);
  useEffect(() => { if (showFinal) finalHeading.current?.focus(); }, [showFinal]);
  let deductionError = '';
  let calculatedWeights: WeightValues | undefined;
  if (value.original) { try { calculatedWeights = adjustedWeights(value.original, finalInput); } catch (error) { deductionError = error instanceof Error ? error.message : '请核对最终重量'; } }
  const calculatedDeduction = value.original && calculatedWeights ? weightDeduction(value.original, calculatedWeights) : undefined;
  const requiresImage = !!value.capture && value.capture.captureRequired !== false && value.capture.imageStatus !== 'disabled';
  const reviewReady = !!value.capture && (!requiresImage || imageReady) && !invalidReason;
  const finalReady = reviewReady && !deductionError;
  const imageCount = value.capture ? value.capture.images?.length || (value.capture.imageSrc ? 1 : 0) : 0;
  const capturePolicy = value.capture?.captureRequired === false ? '非强制抓拍' : '强制抓拍';
  const imageSummary = imageCount ? `${imageCount} 张抓拍` : requiresImage ? '等待抓拍' : '本次无图';

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

        <section className="review-summary" aria-label="本次称重摘要">
          <div className="review-hero review-hero--vehicle">
            <p className="review-hero-label"><Truck className="h-4 w-4" aria-hidden />当前车辆</p>
            <h3 ref={heading} tabIndex={-1} className="review-plate">{value.plate || '待核对'}</h3>
            <div className="review-meta">
              <span className="review-chip">{value.direction}</span>
              <span className="review-chip review-chip--kind">{value.kind}</span>
            </div>
          </div>
          <div className="review-hero review-hero--weight">
            <p className="review-hero-label"><Scale className="h-4 w-4" aria-hidden />本次采集{value.kind}</p>
            <div className="review-weight">{formatWeightValue(value.weight)}<span>吨</span></div>
            <div className="review-weight-meta">
              <span className="review-stable"><CircleCheck className="h-4 w-4" aria-hidden />重量稳定</span>
              <span aria-hidden>·</span>
              <span>{imageSummary}</span>
            </div>
          </div>
          <div className="review-hero review-hero--stage">
            <p className="review-hero-label"><ClipboardCheck className="h-4 w-4" aria-hidden />办理阶段</p>
            <div className="review-stage"><strong>{second ? '回磅' : '首磅'}</strong><span>{second ? '确认后生成正式单据' : '确认后建立运输任务'}</span></div>
            <div className="review-stage-step" aria-label={second ? '第二步，共两步' : '第一步，共两步'}>
              <span className={second ? 'done' : 'active'}>1</span>
              <span className={second ? 'active' : ''}>2</span>
              <span>{second ? '出单' : '回磅'}</span>
            </div>
          </div>
        </section>

        {showFinal ? <section className="review-final-step" aria-label="最终重量与调整">
          <div className="review-final-step-head">
            <span className="review-step-badge"><Info className="h-3.5 w-3.5" aria-hidden />第 2 步 / 共 2 步</span>
            <h3 ref={finalHeading} tabIndex={-1}>最终重量确认</h3>
            <p>请核对原始重量与最终毛重、皮重和净重；确认后将生成正式单据，原始采集依据保留。</p>
          </div>
          <ReviewSection icon={<Scale className="h-4 w-4" aria-hidden />} title="最终重量" extra={<StatusTag tone="info">可调整</StatusTag>}>
            <WeightDeductionFields original={value.original!} value={finalInput} onChange={setFinalInput} showError={false} />
          </ReviewSection>
        </section> : <section className="review-main" aria-label="本趟运输核对依据">
          <ReviewSection icon={<Package className="h-4 w-4" aria-hidden />} title="业务核对" extra={<span className="review-section-extra">请与现场车辆核对</span>}>
            <ReviewSubsection title="基本信息" extra="必填">
              <div className="review-facts">
                <ReviewFact icon={<Package className="h-4 w-4" aria-hidden />} label="物料及规格／编码" value={value.material} wide />
                <ReviewFact icon={<Building2 className="h-4 w-4" aria-hidden />} label={value.partyLabel} value={value.party} wide />
                {value.firstWeight && <ReviewFact icon={<Scale className="h-4 w-4" aria-hidden />} label="首次称重依据" value={value.firstWeight} wide emphasis />}
              </div>
            </ReviewSubsection>
            <ReviewSubsection title="补充运输信息" extra="选填">
              {value.supplementary.length > 0 ? <div className="review-facts review-facts--supplementary">
                {value.supplementary.map((item, index) => <ReviewFact key={`${item.label}-${index}`} icon={supplementaryIcon(item.label)} label={item.label} value={item.value} wide={item.label === '备注'} />)}
              </div> : <p className="review-subsection-empty"><Info className="h-4 w-4" aria-hidden />本次未填写补充运输信息</p>}
            </ReviewSubsection>
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
          {!submitting && requiresImage && !imageReady && <p className="review-footer-info">正在加载本次抓拍图片…</p>}
          {invalidReason && <p className="review-footer-alert">核对已失效，请返回重新采集。</p>}
          {showFinal && deductionError && <p role="alert" className="review-footer-error">无法出单：{deductionError}</p>}
          {submitError && <p className="review-footer-error">保存失败：{submitError}</p>}
          {showFinal && calculatedWeights && calculatedDeduction ? <div className="review-final-summary" aria-live="polite">
            <div className="review-final-item"><span>最终毛重</span><strong>{formatWeightValue(calculatedWeights.gross)}<small>吨</small></strong></div>
            <div className="review-final-item"><span>最终皮重</span><strong>{formatWeightValue(calculatedWeights.tare)}<small>吨</small></strong></div>
            <div className="review-final-item review-final-item--net"><span>最终净重</span><strong>{formatWeightValue(calculatedWeights.net)}<small>吨</small></strong></div>
            <p className="review-deduction-line">系统计算调整差额：毛重 {formatWeight(calculatedDeduction.grossDeduction)} · 皮重 {formatWeight(calculatedDeduction.tareDeduction)}</p>
          </div> : <div className="review-confirm-note"><FileCheck2 className="h-4 w-4" aria-hidden /><span>{second ? '下一步核对最终毛重、皮重和净重；确认后才生成正式单据。' : `确认后保存首磅（${value.kind}）并建立运输任务。`}</span></div>}
        </div>
        <div className="review-actions">
          {showFinal ? <>
            <Button size="lg" disabled={submitting} onClick={() => setStep('review')}><ArrowLeft className="h-4 w-4" aria-hidden />上一步</Button>
            <Button size="lg" variant="primary" className="disabled:opacity-50" disabled={!finalReady || submitting} aria-busy={submitting} onClick={() => { if (finalReady && !submitting) onConfirm(finalInput); }}>
              <Check className="h-4 w-4" aria-hidden />{submitting ? '正在保存…' : '确认称重并出单'}
            </Button>
          </> : <>
            <Button size="lg" disabled={submitting} onClick={invalidReason ? onCancel : onMismatch}><ArrowLeft className="h-4 w-4" aria-hidden />{invalidReason ? '返回重新采集' : '不一致，返回修改'}</Button>
            {second ? <Button size="lg" variant="primary" className="disabled:opacity-50" disabled={!reviewReady || submitting} onClick={() => { if (reviewReady && !submitting) setStep('final'); }}>
              <ArrowRight className="h-4 w-4" aria-hidden />下一步：核对最终重量
            </Button> : <Button size="lg" variant="primary" className="disabled:opacity-50" disabled={!reviewReady || submitting} onClick={() => { if (reviewReady && !submitting) onConfirm(finalInput); }}>
              <Check className="h-4 w-4" aria-hidden />确认首磅（{value.kind}）
            </Button>}
          </>}
        </div>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
