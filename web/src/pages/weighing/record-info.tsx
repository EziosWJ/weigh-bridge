import { useTransportDirections } from '@/hooks/use-transport-directions';
import { DetailItem } from "@/components/common/detail-item";
import { beijingTime } from "@/lib/weighing/query";
import { formatWeight } from "@/lib/weighing/weights";
import { roleLabels, partyNatureLabels, supplementReasonLabels, recordSourceLabel, type BusinessInfo, type WeighRecord } from "@/types/weighing";
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

function RecordTimes({ record }: { record: WeighRecord }) {
  const manual = record.source === 'manual';
  const capturedAt = record.capturedAt || (!manual ? record.capture?.at : undefined);
  return <div className="grid gap-space-3 py-space-2 sm:grid-cols-3">
    <DetailItem label={manual ? (record.stage === 'first' ? '实际首磅时间' : '实际回磅时间') : '设备采集时间'} value={capturedAt ? beijingTime(capturedAt) : undefined} />
    <DetailItem label={manual ? '系统补录时间' : '系统确认时间'} value={beijingTime(record.at)} />
    <DetailItem label={manual ? '补录人' : '确认人'} value={record.actor ? [record.actor, record.role && roleLabels[record.role]].filter(Boolean).join(' / ') : undefined} />
  </div>;
}

export function WeighRecords({ records, entryType, onPreviewChange }: { records: WeighRecord[]; entryType?: string; onPreviewChange?: (open: boolean) => void }) {
  return <div className="space-y-space-3">
    {records.length === 0 && <p className="text-text-tertiary">尚未确认称重</p>}
    {records.map(record => <div key={record.id} className="rounded-control border border-border p-space-3 text-sm">
      <div className="flex flex-wrap justify-between gap-space-2">
        <strong>{record.stage === 'first' ? '首次' : '二次'} · {record.kind === 'gross' ? '毛重' : '皮重'} {formatWeight(record.weight)}</strong>
        <span className="text-text-secondary">{recordSourceLabel(record.stage, record.source, entryType)} · {records.some(next => next.replaces === record.id) ? '已替代' : '当前记录'}</span>
      </div>
      <RecordTimes record={record} />
      {record.source !== 'manual' && <div className="my-space-3 max-w-sm">{record.capture ? <CapturePhoto capture={record.capture} onPreviewChange={onPreviewChange} /> : <p className="text-text-tertiary">无抓拍记录</p>}</div>}
      <div className="space-y-1 break-words text-text-secondary">
        <p>记录 {record.id}</p>
        {record.retainedRecordId && <p>沿用另一有效磅 {record.retainedRecordId}</p>}
        {record.replaces && <p>替代记录 {record.replaces}</p>}
        {record.reason && <p>{record.source === 'manual' ? '补录原因' : '操作原因'}：{supplementReasonLabels[record.reason] ?? record.reason}</p>}
      </div>
    </div>)}
  </div>;
}
