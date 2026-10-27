import { useCallback, useEffect, useState } from "react";
import { weighingApi } from "@/api/weighing";
import type { DailyTicketSequence } from "@/api/weighing";
import { ContentCard } from "@/components/common/content-card";
import { toast } from "@/components/common/toast-store";
import { Field } from "@/components/common/field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { getErrorMessage } from "@/lib/api-error";

function beijingToday(): string {
  // en-CA renders YYYY-MM-DD; the business date is always Beijing time.
  return new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Shanghai", year: "numeric", month: "2-digit", day: "2-digit" })
    .format(new Date())
    .replace(/-/g, "");
}

export function TicketSequenceConfiguration() {
  const [allowed, setAllowed] = useState(false);
  const [rows, setRows] = useState<DailyTicketSequence[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [businessDate, setBusinessDate] = useState(beijingToday());
  const [lastIssued, setLastIssued] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const capabilities = await weighingApi.capabilities();
      // The backend accepts either the configuration-management permission
      // (this page) or the retired site-management permission (ADR 0033).
      if (!capabilities.permissionCodes.some(code => code === "system:config" || code === "weighing:site:manage")) {
        setAllowed(false);
        return;
      }
      setAllowed(true);
      setRows(await weighingApi.ticketSequences());
    } catch (failure) {
      setError(getErrorMessage(failure, "流水号读取失败"));
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => { void load(); }, [load]);

  async function submit() {
    if (saving) return;
    const date = businessDate.trim();
    if (!/^\d{8}$/.test(date)) {
      setError("请填写 8 位北京时间出单日期，例如 20261027。");
      return;
    }
    const raw = lastIssued.trim();
    if (!/^\d+$/.test(raw)) {
      setError("请填写该日期已发放的最大流水号，例如 50。");
      return;
    }
    if (!window.confirm(`将 ${date} 的已发最大流水号设为 ${raw}，之后从 ${Number(raw) + 1} 继续发号。此操作只升不降并记录审计，仅在恢复备份并核实已发单号后使用。是否继续？`)) return;
    setSaving(true);
    setError("");
    try {
      const saved = await weighingApi.setTicketWatermark({ businessDate: date, lastIssued: Number(raw) });
      toast.success({ title: "流水号已更新", description: `${saved.businessDate} 已发最大流水号 ${saved.lastIssued}，下一张从 ${saved.lastIssued + 1} 开始。` });
      setLastIssued("");
      await load();
    } catch (failure) {
      setError(getErrorMessage(failure, "流水号更新失败"));
    } finally {
      setSaving(false);
    }
  }

  if (!loading && !allowed) return null;
  return <ContentCard
    title="流水号水位"
    description="仅在恢复备份后使用：先核实库外已发纸单或云端单号，再把对应日期的已发最大流水号设置到安全位置，确认后才恢复出单。只允许上调并记录审计，历史单号不改写。"
    className="mb-space-6"
  >
    {loading ? <p role="status" className="text-sm text-text-tertiary">正在加载流水号…</p> : (
      <form onSubmit={(event) => { event.preventDefault(); void submit(); }}>
        <div className="grid w-full max-w-2xl gap-space-3 [&_p:empty]:hidden sm:grid-cols-2">
          <Field label="出单日期（北京时间）" htmlFor="ticket-sequence-date" help="8 位日期，例如 20261027">
            <Input id="ticket-sequence-date" value={businessDate} maxLength={8} disabled={saving} onChange={(event) => { setBusinessDate(event.target.value); setError(""); }} />
          </Field>
          <Field label="已发最大流水号" htmlFor="ticket-sequence-last" help="只升不降；下一张从该值加 1 开始">
            <Input id="ticket-sequence-last" value={lastIssued} maxLength={12} disabled={saving} placeholder="例如：50" onChange={(event) => { setLastIssued(event.target.value); setError(""); }} />
          </Field>
        </div>
        <div className="mt-space-3">
          <Button type="submit" variant="primary" disabled={saving || !lastIssued.trim()}>{saving ? "正在保存…" : "设置水位"}</Button>
        </div>
      </form>
    )}
    {rows.length > 0 && <div className="mt-space-3 max-w-2xl">
      <p className="mb-space-2 text-sm text-text-tertiary">最近的已分配流水号：</p>
      <ul className="flex flex-wrap gap-space-2 text-sm" aria-label="最近的流水号水位">
        {rows.map((row) => <li key={row.businessDate} className="rounded border border-border-default px-space-2 py-space-1">
          {row.businessDate} · 已发 {row.lastIssued}
        </li>)}
      </ul>
    </div>}
    {error && <p role="alert" className="mt-space-3 text-sm text-error">{error}</p>}
  </ContentCard>;
}
