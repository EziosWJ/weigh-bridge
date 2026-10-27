import { useCallback, useEffect, useState } from "react";
import { getSystemConfigPage, updateSystemConfig } from "@/api/system";
import { ContentCard } from "@/components/common/content-card";
import { DetailDialog } from "@/components/common/detail-dialog";
import { toast } from "@/components/common/toast-store";
import { Field } from "@/components/common/field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { getErrorMessage } from "@/lib/api-error";
import { TicketSheet } from "@/pages/weighing/ticket-sheet";
import type { SystemConfigRecord } from "@/types";

const titleKey = "weighing.ticket-title";

export function TicketTitleConfiguration() {
  const [record, setRecord] = useState<SystemConfigRecord | null>(null);
  const [title, setTitle] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [preview, setPreview] = useState(false);
  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const result = await getSystemConfigPage({ page: 1, pageSize: 10, configKey: titleKey });
      const row = result.records.find((item) => item.configKey === titleKey);
      if (!row) throw new Error("未找到过磅单标题配置，请检查数据库迁移及服务启动日志。");
      setRecord(row);
      setTitle(row.configValue?.trim() || "酒厂过磅单");
    } catch (failure) {
      setRecord(null);
      setError(getErrorMessage(failure, "过磅单标题加载失败"));
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => { void load(); }, [load]);

  async function save() {
    if (!record || saving) return;
    const value = title.trim();
    if (!value || [...value].length > 100 || /[\r\n\t]/.test(value)) {
      setError("请填写 1～100 个字符的单行标题。");
      return;
    }
    setSaving(true);
    setError("");
    try {
      await updateSystemConfig(record.id, {
        configName: record.configName ?? "过磅单标题", configKey: titleKey, configValue: value,
        configType: record.configType ?? "SYSTEM", valueType: "TEXT", status: 1, remark: record.remark ?? undefined,
      });
      setRecord({ ...record, configValue: value });
      setTitle(value);
      toast.success({ title: "过磅单标题已保存", description: "新标题将用于之后生成的单据；历史单据保持不变。" });
    } catch (failure) {
      setError(getErrorMessage(failure, "过磅单标题保存失败"));
    } finally {
      setSaving(false);
    }
  }

  return <ContentCard title="过磅单标题" description="修改后用于新生成的单据；历史单据和补打保留原标题。" className="mb-space-6">
    {loading ? <p role="status" className="text-sm text-text-tertiary">正在加载过磅单标题…</p> : record ? (
      <form onSubmit={(event) => { event.preventDefault(); void save(); }}>
        <div className="flex flex-wrap items-end gap-space-3">
          <div className="w-full max-w-xl [&_p:empty]:hidden">
            <Field label="打印标题" htmlFor="ticket-title">
              <Input id="ticket-title" className="scroll-mt-24" value={title} maxLength={100} disabled={saving} placeholder="例如：某某酒厂过磅单" onChange={(event) => { setTitle(event.target.value); setError(""); }} />
            </Field>
          </div>
          <Button type="button" disabled={saving || !title.trim()} onClick={() => setPreview(true)}>预览标题</Button>
          <Button type="submit" variant="primary" disabled={saving}>{saving ? "正在保存…" : "保存标题"}</Button>
        </div>
      </form>
    ) : <Button onClick={() => void load()}>重新加载标题</Button>}
    {error && <p role="alert" className="mt-space-3 text-sm text-error">{error}</p>}
    <DetailDialog open={preview} title="过磅单标题预览" description="使用当前输入的标题展示样张，保存后生效。预览不会打印或生成正式单据。" onCancel={() => setPreview(false)} trapFocus>
      <TicketSheet sheet={{ template: "weighing-compact-v2", widthMm: 241, heightMm: 93, layers: 5, title: title.trim(), marks: ["标题预览样张"], rows: [
        [{ label: "单号", value: "F001-20261027-0001" }],
        [{ label: "车牌", value: "川A12345" }, { label: "业务", value: "入厂卸货" }],
        [{ label: "供货方", value: "示例供货方" }, { label: "物料", value: "高粱 / 一级" }],
        [{ label: "毛重（吨）", value: "30.125" }, { label: "皮重（吨）", value: "10" }, { label: "净重（吨）", value: "20.125" }],
        [{ label: "首次称重", value: "2026-10-26 08:00:00" }, { label: "二次称重", value: "2026-10-26 09:00:00" }],
        [{ label: "操作员", value: "示例操作员" }, { label: "司机签字", value: "________________" }],
      ] }} />
    </DetailDialog>
  </ContentCard>;
}
