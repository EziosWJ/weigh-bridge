import { useState } from "react";
import { Field } from "@/components/common/field";
import { FormSection } from "@/components/common/form-section";
import { PageHeader } from "@/components/common/page-header";
import { RecordPicker, type RecordInfo } from "@/components/common/record-picker";
import type { ApiPageResult } from "@/types/api";

type DemoRecord = {
  id: number;
  name: string;
  code: string;
  model: string;
  unit: string;
};

const models = ["标准型", "紧凑型", "增强型", "便携型"];
const units = ["件", "箱", "套"];

const records: DemoRecord[] = Array.from({ length: 24 }, (_, index) => {
  const seq = index + 1;

  return {
    id: seq,
    name: `演示档案 ${String(seq).padStart(2, "0")}`,
    code: `DEMO-${String(seq).padStart(3, "0")}`,
    model: models[index % models.length],
    unit: units[index % units.length],
  };
});

function describe(record: DemoRecord): RecordInfo {
  return {
    name: record.name,
    details: [record.model, `单位：${record.unit}`],
    tags: ["演示档案"],
    code: record.code,
  };
}

function wait(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

async function loadPage(
  keyword: string,
  page: number,
  size: number,
): Promise<ApiPageResult<DemoRecord>> {
  await wait(180);
  const matched = records.filter(
    (record) =>
      !keyword ||
      record.name.includes(keyword) ||
      record.code.includes(keyword) ||
      record.model.includes(keyword),
  );

  return {
    records: matched.slice((page - 1) * size, page * size),
    total: matched.length,
    page,
    pageSize: size,
  };
}

/** 本 Demo 只演示选择本身，不引入停用或历史资料警示，因此所有档案恒可选择。 */
function warning() {
  return "";
}

async function loadRecord(id: number) {
  await wait(120);
  const record = records.find((item) => item.id === id);
  if (!record) throw new Error("演示档案不存在，请重新选择。");

  return record;
}

export function BrowsePickerDemoPage() {
  const [value, setValue] = useState(0);
  const selected = records.find((record) => record.id === value) ?? null;

  return (
    <>
      <PageHeader
        title="浏览选择 Demo"
        description="档案选择示例，展示输入框尾部的浏览入口、关键词候选与完整分页面板。"
      />

      <div className="max-w-[1080px] space-y-6">
        <FormSection
          title="档案选择"
          description="字段可直接输入搜索，也可点尾部图标浏览全部档案。"
        >
          <Field
            label="演示档案"
            help="可直接输入名称、编码或型号搜索；有内容时尾部图标左侧会出现清除按钮。"
          >
            <div className="max-w-[420px]">
              <RecordPicker<DemoRecord>
                label="演示档案"
                placeholder="输入名称、编码或型号"
                value={value}
                loadPage={loadPage}
                loadRecord={loadRecord}
                describe={describe}
                warning={warning}
                onChange={(record) => setValue(record?.id ?? 0)}
              />
            </div>
          </Field>

          <div className="space-y-1.5">
            <p className="block text-form-label font-medium text-text-primary">
              当前选择
            </p>
            {selected ? (
              <div className="space-y-1 rounded-admin border border-border bg-surface px-4 py-3 text-sm shadow-admin">
                <p className="font-medium text-text-primary">
                  {describe(selected).name}
                </p>
                <p className="text-text-secondary">
                  {describe(selected).details.join("；")}
                </p>
                <p className="text-text-tertiary">
                  {describe(selected).tags.join("，")} · {describe(selected).code}
                </p>
              </div>
            ) : (
              <p className="rounded-admin border border-dashed border-border px-4 py-3 text-sm text-text-tertiary">
                尚未选择，点尾部图标浏览全部档案，或直接输入关键词。
              </p>
            )}
          </div>
        </FormSection>

        <div className="rounded-admin border border-border bg-surface px-5 py-4 shadow-admin">
          <p className="text-sm font-medium text-text-primary">四要点对照</p>
          <ul className="mt-2 space-y-1.5 text-sm text-text-tertiary">
            <li>① 输入框可直接输入：聚焦后输入关键词，候选中命中的名称、辨认资料与编码会高亮。</li>
            <li>② 输入框尾部的列表图标是浏览入口，可用 Tab 聚焦，带悬停说明与可访问名称「浏览选择演示档案」。</li>
            <li>③ 有文字或有值时，输入框内出现独立的清除按钮（「清除演示档案」），与文字、浏览图标互不遮挡。</li>
            <li>④ 聚焦即弹出快速候选（最多 6 条），「查看全部结果（24）」打开 800px 宽的居中分页面板，每页 10 条。</li>
          </ul>
          <p className="mt-3 text-sm text-text-tertiary">
            本页为静态演示，档案数据来自页面内存，不请求后端接口。
          </p>
        </div>
      </div>
    </>
  );
}
