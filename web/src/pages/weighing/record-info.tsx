import { useTransportDirections } from '@/hooks/use-transport-directions';
import { DetailItem } from "@/components/common/detail-item";
import { beijingTime } from "@/lib/weighing/query";
import { formatWeight } from "@/lib/weighing/weights";
import { roleLabels, partyNatureLabels, type BusinessInfo, type WeighRecord } from "@/types/weighing";
import { CapturePhoto } from "./camera-panel";

/** 只展示业务发生时的快照，旧记录缺少的字段不从当前资料反填。 */
export function BusinessSnapshot({ value }: { value: BusinessInfo }) {
  const directionLabels = useTransportDirections();
  return <div className="space-y-space-5">
    <div className="grid gap-space-4 sm:grid-cols-2 xl:grid-cols-3">
      <DetailItem label="车牌" value={value.plate} />
      <DetailItem label="运输方向" value={directionLabels[value.direction]} />
      <DetailItem label="物料名称" value={value.materialName} />
      <DetailItem label="物料编码" value={value.materialCode} />
      <DetailItem label="规格 / 等级" value={value.materialSpec} />
      <DetailItem label="物料分类" value={value.materialCategoryName} />
    </div>
    <div className="grid gap-space-4 border-t border-border pt-space-4 sm:grid-cols-2 xl:grid-cols-3">
      <DetailItem label={value.direction === "inbound" ? "供货方" : "收货方"} value={value.partyName} />
      <DetailItem label="供收方编码" value={value.partyCode} />
      <DetailItem label="主体性质" value={value.partyNature ? partyNatureLabels[value.partyNature] : undefined} />
      <DetailItem label="主体联系电话" value={value.partyPhone} />
      <DetailItem label="业务地址" value={value.partyAddress} />
      <DetailItem label="承运方" value={value.carrierName} />
      <DetailItem label="承运方编码" value={value.carrierCode} />
    </div>
    <div className="grid gap-space-4 border-t border-border pt-space-4 sm:grid-cols-2 xl:grid-cols-3">
      <DetailItem label="本趟联系人" value={value.contactName} />
      <DetailItem label="本趟联系电话" value={value.contactPhone} />
      <DetailItem label="司机姓名" value={value.driver} />
      <DetailItem label="司机电话" value={value.driverPhone} />
      <DetailItem label="备注" value={value.remark} />
    </div>
  </div>;
}

export function ManualReturnNotice({ records }: { records: WeighRecord[] }) {
  const manual = records.filter(record => record.source === 'manual');
  if (!manual.length) return null;
  return <div role="note" className="rounded-control border border-warning-border bg-warning-background p-space-3 text-sm text-warning">
    <p className="font-medium">本单含人工补录回磅，不是设备采集结果。</p>
    {manual.map(record => <p key={record.id} className="mt-space-1 break-words">
      记录 {record.id} · {record.kind === 'gross' ? '毛重' : '皮重'} {formatWeight(record.weight)} · 实际回磅时间 {beijingTime(record.capturedAt ?? record.at)} · 系统补录时间 {beijingTime(record.at)} · 补录人 {record.actor || '历史未记录'}{record.reason ? ` · 原因：${record.reason}` : ''}。本流程不要求系统抓拍，也未保存照片依据。
    </p>)}
  </div>;
}

export function WeighRecords({ records, onPreviewChange }: { records: WeighRecord[]; onPreviewChange?: (open: boolean) => void }) {
  return <div className="space-y-space-3">
    {records.length === 0 && <p className="text-text-tertiary">尚未确认称重。</p>}
    {records.map((record) => <div key={record.id} className="rounded-control border border-border p-space-3 text-sm">
      <div className="flex flex-wrap justify-between gap-space-2"><strong>{record.stage === "first" ? "首次" : "二次"} · {record.kind === "gross" ? "毛重" : "皮重"} {formatWeight(record.weight)}</strong><span>{record.source === 'manual' ? `实际回磅 ${beijingTime(record.capturedAt ?? record.at)}` : beijingTime(record.at)}</span></div>
      <p className="mt-space-2 break-all text-text-secondary">记录 {record.id} · {record.source === 'manual' ? '人工补录（非设备采集）' : '采集依据'}{records.some((next) => next.replaces === record.id) ? " · 已被后续记录替代" : " · 当前记录"}</p>
      {record.source === 'manual'
        ? <><p className="mt-1 text-text-secondary">补录人：{record.actor || '历史未记录'} · 系统补录时间：{beijingTime(record.at)} · 实际回磅时间：{beijingTime(record.capturedAt ?? record.at)}</p><div className="mt-space-3"><p className="text-text-tertiary">人工补录不要求系统抓拍，也没有设备采集或抓拍图片。</p></div></>
        : <><p className="mt-1 text-text-secondary">操作人：{record.actor || '历史未记录'} · 岗位：{record.role ? roleLabels[record.role] : '历史未记录'} · 采集时间：{record.capture ? beijingTime(record.capture.at) : '历史未记录'}</p><div className="mt-space-3 max-w-sm">{record.capture ? <CapturePhoto capture={record.capture} onPreviewChange={onPreviewChange} /> : <p className="text-text-tertiary">旧记录未保存抓拍图片，不能用当前示例反填。</p>}</div></>}
      {record.retainedRecordId && <p className="mt-1">沿用另一有效磅：{record.retainedRecordId}</p>}
      {record.replaces && <p className="mt-space-1 break-all">替代原记录：{record.replaces}</p>}
      {record.reason && <p className="mt-space-1">原因：{record.reason}</p>}
    </div>)}
  </div>;
}
