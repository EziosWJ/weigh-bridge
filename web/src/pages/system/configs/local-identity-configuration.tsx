import { useCallback, useEffect, useState } from "react";
import { weighingApi } from "@/api/weighing";
import type { WeighingCapabilities } from "@/api/weighing";
import { ContentCard } from "@/components/common/content-card";
import { toast } from "@/components/common/toast-store";
import { Field } from "@/components/common/field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { getErrorMessage } from "@/lib/api-error";

type LocalIdentity = { code: string; name: string; version: number; scaleCode: string; scaleName: string; agentId: string; enabled: number };

export function LocalIdentityConfiguration() {
  const [identity, setIdentity] = useState<LocalIdentity | null>(null);
  const [capabilities, setCapabilities] = useState<WeighingCapabilities | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  // Initialization form (unconfigured factory): head-office assigned codes.
  const [siteCode, setSiteCode] = useState("");
  const [initScaleCode, setInitScaleCode] = useState("WB01");
  const [initName, setInitName] = useState("");
  const [initScaleName, setInitScaleName] = useState("");
  // Name maintenance (configured factory).
  const [name, setName] = useState("");
  const [scaleName, setScaleName] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const caps = await weighingApi.capabilities();
      setCapabilities(caps);
      if (!caps.configured) return;
      const rows = (await weighingApi.siteConfiguration()) as LocalIdentity[];
      const row = rows[0];
      if (!row) throw new Error("未读取到本厂身份，请检查数据库迁移及服务日志。");
      setIdentity(row);
      setSiteCode(row.code);
      setInitScaleCode(row.scaleCode);
      setName(row.name);
      setScaleName(row.scaleName);
    } catch (failure) {
      setError(getErrorMessage(failure, "本厂身份加载失败"));
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => { void load(); }, [load]);

  async function initialize() {
    if (saving) return;
    const code = siteCode.trim();
    if (!/^[A-Za-z0-9_.-]{1,40}$/.test(code)) {
      setError("厂区编码由总部预先分配，1～40 位字母、数字、点、下划线或连字符。");
      return;
    }
    setSaving(true);
    setError("");
    try {
      await weighingApi.initializeLocalSite({ siteCode: code, scaleCode: initScaleCode.trim() || "WB01", name: initName.trim() || undefined, scaleName: initScaleName.trim() || undefined });
      toast.success({ title: "本厂身份已配置", description: "本厂身份已保存。" });
      await load();
    } catch (failure) {
      setError(getErrorMessage(failure, "本厂身份配置失败"));
    } finally {
      setSaving(false);
    }
  }

  async function save() {
    if (!identity || saving) return;
    const nextName = name.trim();
    const nextScaleName = scaleName.trim();
    const nextSiteCode = siteCode.trim();
    const nextScaleCode = initScaleCode.trim();
    if (!/^[A-Za-z0-9_.-]{1,40}$/.test(nextSiteCode) || !/^[A-Za-z0-9_.-]{1,40}$/.test(nextScaleCode)) {
      setError("厂区编码和地磅编码须为 1～40 位字母、数字、点、下划线或连字符。");
      return;
    }
    if (!nextName || !nextScaleName) {
      setError("厂区名称与地磅名称不能为空。");
      return;
    }
    setSaving(true);
    setError("");
    try {
      await weighingApi.saveSiteConfiguration({ code: nextSiteCode, scaleCode: nextScaleCode, name: nextName, scaleName: nextScaleName, enabled: identity.enabled === 0 ? 0 : 1, version: identity.version });
      toast.success({ title: "本厂身份已保存", description: "历史单据保持原编号；编码变更后设备将重新连接。" });
      await load();
    } catch (failure) {
      setError(getErrorMessage(failure, "本厂身份保存失败"));
    } finally {
      setSaving(false);
    }
  }

  const manage = capabilities?.permissionCodes.some(code => code === "system:config" || code === "weighing:site:manage") ?? false;
  if (!loading && !manage) return null;
  const codePattern = "1～40 位字母、数字、点、下划线或连字符";
  return <ContentCard title="本厂身份" description="厂区编码由总部预先分配；编码修改后，历史单据保留原编号，设备将重新连接。" className="mb-space-6">
    {loading ? <p role="status" className="text-sm text-text-tertiary">正在加载本厂身份…</p>
    : capabilities && !capabilities.configured ? (
      <form onSubmit={(event) => { event.preventDefault(); void initialize(); }}>
        <div className="grid w-full max-w-2xl gap-space-3 [&_p:empty]:hidden sm:grid-cols-2">
          <Field label="厂区编码" htmlFor="local-site-code" help={codePattern}>
            <Input id="local-site-code" value={siteCode} maxLength={40} disabled={saving} placeholder="例如：F001" onChange={(event) => { setSiteCode(event.target.value); setError(""); }} />
          </Field>
          <Field label="地磅编码" htmlFor="local-scale-code" help="默认 WB01">
            <Input id="local-scale-code" value={initScaleCode} maxLength={40} disabled={saving} onChange={(event) => { setInitScaleCode(event.target.value); setError(""); }} />
          </Field>
          <Field label="厂区名称" htmlFor="local-site-name" help="选填，默认“本厂”">
            <Input id="local-site-name" value={initName} maxLength={100} disabled={saving} placeholder="本厂" onChange={(event) => setInitName(event.target.value)} />
          </Field>
          <Field label="地磅名称" htmlFor="local-scale-name" help="选填，默认“本厂地磅”">
            <Input id="local-scale-name" value={initScaleName} maxLength={100} disabled={saving} placeholder="本厂地磅" onChange={(event) => setInitScaleName(event.target.value)} />
          </Field>
        </div>
        <div className="mt-space-3">
          <Button type="submit" variant="primary" disabled={saving || !siteCode.trim()}>{saving ? "正在配置…" : "确定本厂身份"}</Button>
        </div>
      </form>
    ) : identity ? (
      <form onSubmit={(event) => { event.preventDefault(); void save(); }}>
        <div className="grid w-full max-w-2xl gap-space-3 [&_p:empty]:hidden sm:grid-cols-2">
          <Field label="厂区编码" htmlFor="local-identity-site-code" help={codePattern}>
            <Input id="local-identity-site-code" value={siteCode} maxLength={40} disabled={saving} onChange={(event) => { setSiteCode(event.target.value); setError(""); }} />
          </Field>
          <Field label="地磅编码" htmlFor="local-identity-scale-code" help={codePattern}>
            <Input id="local-identity-scale-code" value={initScaleCode} maxLength={40} disabled={saving} onChange={(event) => { setInitScaleCode(event.target.value); setError(""); }} />
          </Field>
          <Field label="厂区名称" htmlFor="local-identity-site-name">
            <Input id="local-identity-site-name" value={name} maxLength={100} disabled={saving} onChange={(event) => { setName(event.target.value); setError(""); }} />
          </Field>
          <Field label="地磅名称" htmlFor="local-identity-scale-name">
            <Input id="local-identity-scale-name" value={scaleName} maxLength={100} disabled={saving} onChange={(event) => { setScaleName(event.target.value); setError(""); }} />
          </Field>
          <Field label="启停状态" htmlFor="local-identity-enabled" help="停用与启用沿用既有业务检查，本页只读展示">
            <Input id="local-identity-enabled" value={identity.enabled === 1 ? "启用" : "停用"} readOnly disabled aria-readonly />
          </Field>
        </div>
        <div className="mt-space-3">
          <Button type="submit" variant="primary" disabled={saving}>{saving ? "正在保存…" : "保存身份"}</Button>
        </div>
      </form>
    ) : <Button onClick={() => void load()}>重新加载本厂身份</Button>}
    {error && <p role="alert" className="mt-space-3 text-sm text-error">{error}</p>}
  </ContentCard>;
}
