import { useCallback, useEffect, useState } from "react";
import {
  siteConfigurationApi,
  type LocalSiteStatus,
  type SiteConfiguration,
} from "@/api/site-configuration";
import { ContentCard } from "@/components/common/content-card";
import { DetailItem } from "@/components/common/detail-item";
import { EmptyState } from "@/components/common/empty-state";
import { FormDialog } from "@/components/common/form-dialog";
import { Field } from "@/components/common/field";
import { StatusTag } from "@/components/common/status-tag";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { getErrorMessage } from "@/lib/api-error";

type InitialForm = { name: string; scaleName: string };
const emptyInitialForm: InitialForm = { name: "", scaleName: "" };

type LocalSiteConfigurationProps = {
  canManage: boolean;
};

/**
 * The local identity group inside 系统管理 → 配置管理. It edits only the two
 * business names (and the enabled state); internal codes and the Agent
 * association are never shown or typed by an operator.
 */
export function LocalSiteConfiguration({
  canManage,
}: LocalSiteConfigurationProps) {
  const [status, setStatus] = useState<LocalSiteStatus | null>(null);
  const [row, setRow] = useState<SiteConfiguration | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [editing, setEditing] = useState<SiteConfiguration | null>(null);
  const [initialForm, setInitialForm] = useState<InitialForm>(emptyInitialForm);
  const [initializing, setInitializing] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    setLoadError("");
    try {
      const [rows, localStatus] = await Promise.all([
        siteConfigurationApi.list(),
        siteConfigurationApi.localStatus(),
      ]);
      setStatus(localStatus);
      setRow(
        localStatus.configured
          ? rows.find((item) => item.id === localStatus.siteId) ?? null
          : null,
      );
    } catch (loadFailure) {
      setStatus(null);
      setRow(null);
      setLoadError(getErrorMessage(loadFailure, "本厂配置加载失败"));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const openInitialForm = () => {
    setError("");
    setInitialForm(emptyInitialForm);
    setInitializing(true);
  };

  const openEditForm = () => {
    if (!row) return;
    setError("");
    setEditing({ ...row });
  };

  const initialize = async () => {
    if (!canManage || !status || status.configured) return;
    const name = initialForm.name.trim();
    const scaleName = initialForm.scaleName.trim();
    if (!name || !scaleName) {
      setError("请填写厂区名称和地磅名称。");
      return;
    }
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await siteConfigurationApi.initialize({ name, scaleName });
      setInitializing(false);
      setInitialForm(emptyInitialForm);
      setNotice("本厂配置已完成。");
      await refresh();
    } catch (initializeFailure) {
      setError(getErrorMessage(initializeFailure, "本厂首次配置失败"));
    } finally {
      setBusy(false);
    }
  };

  const save = async () => {
    if (!editing || !canManage) return;
    const name = editing.name.trim();
    const scaleName = editing.scaleName.trim();
    if (!name || !scaleName) {
      setError("请填写厂区名称和地磅名称。");
      return;
    }
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await siteConfigurationApi.save(editing.id, {
        name,
        scaleName,
        enabled: editing.enabled,
        version: editing.version,
      });
      setEditing(null);
      setNotice("本厂配置已保存。");
      await refresh();
    } catch (saveFailure) {
      setError(getErrorMessage(saveFailure, "本厂配置保存失败"));
    } finally {
      setBusy(false);
    }
  };

  const configured = Boolean(status?.configured);
  return (
    <>
      <ContentCard
        className="mb-space-6"
        title="本厂配置"
        description="维护本厂与地磅名称；编号与设备关联由系统内部管理。"
        extra={
          <div className="flex flex-wrap items-center justify-end gap-space-2">
            {!loading && !loadError && (
              <StatusTag tone={configured ? "success" : "warning"}>
                {configured ? (row?.enabled === 1 ? "已配置" : "已停用") : "未配置"}
              </StatusTag>
            )}
            <Button
              size="sm"
              variant="secondary"
              disabled={busy || loading}
              onClick={() => void refresh()}
            >
              刷新
            </Button>
            {!configured && canManage && !loadError && (
              <Button
                size="sm"
                variant="primary"
                disabled={busy || loading}
                onClick={openInitialForm}
              >
                首次配置
              </Button>
            )}
            {configured && row && canManage && (
              <Button
                size="sm"
                variant="primary"
                disabled={busy || loading}
                onClick={openEditForm}
              >
                编辑本厂配置
              </Button>
            )}
          </div>
        }
      >
        {loadError && (
          <p role="alert" className="text-sm text-error">
            {loadError}
          </p>
        )}
        {notice && (
          <p role="status" className="mb-space-3 text-sm text-text-secondary">
            {notice}
          </p>
        )}
        {loading && !loadError ? (
          <p role="status" className="text-sm text-text-tertiary">
            正在加载本厂配置…
          </p>
        ) : null}
        {!loading && !loadError && !configured ? (
          <EmptyState
            title="本厂尚未完成首次配置"
            description={
              canManage
                ? "填写厂区名称和地磅名称即可；保存成功即完成配置。"
                : "请联系具备本厂配置权限的管理员完成首次配置。"
            }
          />
        ) : null}
        {!loading && !loadError && configured && !row ? (
          <EmptyState
            title="未找到本厂配置记录"
            description="本地身份状态与配置记录不一致，请联系系统管理员检查本地数据库。"
          />
        ) : null}
        {!loading && !loadError && row ? (
          <div className="space-y-space-4">
            <div className="grid gap-space-4 sm:grid-cols-3">
              <DetailItem label="厂区名称" value={row.name} />
              <DetailItem label="地磅名称" value={row.scaleName} />
              <DetailItem
                label="状态"
                value={<StatusTag tone={row.enabled ? "success" : "neutral"}>{row.enabled ? "启用" : "停用"}</StatusTag>}
              />
            </div>
            <p className="text-sm text-text-tertiary">
              配置完成不代表设备已就绪；设备连接与运行状态以设备维护页为准。
            </p>
            {!canManage && (
              <p className="text-sm text-text-secondary">当前账号仅可查看本厂配置。</p>
            )}
          </div>
        ) : null}
      </ContentCard>

      <FormDialog
        open={initializing}
        title="首次配置本厂"
        description="填写本厂与地磅名称即可；厂区、地磅及内部关联由系统生成，保存成功后固定。"
        loading={busy}
        submitText="保存并初始化"
        onCancel={() => setInitializing(false)}
        onSubmit={initialize}
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="厂区名称" htmlFor="initial-name" required>
            <Input
              id="initial-name"
              type="text"
              required
              disabled={busy}
              maxLength={100}
              value={initialForm.name}
              onChange={(event) =>
                setInitialForm({ ...initialForm, name: event.target.value })
              }
            />
          </Field>
          <Field label="地磅名称" htmlFor="initial-scaleName" required>
            <Input
              id="initial-scaleName"
              type="text"
              required
              disabled={busy}
              maxLength={100}
              value={initialForm.scaleName}
              onChange={(event) =>
                setInitialForm({ ...initialForm, scaleName: event.target.value })
              }
            />
          </Field>
        </div>
        {error && (
          <p role="alert" className="mt-3 text-sm text-error">
            {error}
          </p>
        )}
      </FormDialog>

      <FormDialog
        open={Boolean(editing)}
        title="编辑本厂配置"
        description="厂区与地磅名称可维护；内部编号与设备关联不在此编辑。停用前须结束任务、核实打印并停止 Agent。"
        loading={busy}
        onCancel={() => setEditing(null)}
        onSubmit={save}
      >
        {editing && (
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="厂区名称" htmlFor="site-name" required>
              <Input
                id="site-name"
                type="text"
                required
                disabled={busy}
                maxLength={100}
                value={editing.name}
                onChange={(event) =>
                  setEditing({ ...editing, name: event.target.value })
                }
              />
            </Field>
            <Field label="地磅名称" htmlFor="site-scaleName" required>
              <Input
                id="site-scaleName"
                type="text"
                required
                disabled={busy}
                maxLength={100}
                value={editing.scaleName}
                onChange={(event) =>
                  setEditing({ ...editing, scaleName: event.target.value })
                }
              />
            </Field>
            <Field label="状态" htmlFor="site-enabled">
              <Select
                id="site-enabled"
                disabled={busy}
                value={String(editing.enabled)}
                onChange={(event) =>
                  setEditing({
                    ...editing,
                    enabled: Number(event.target.value),
                  })
                }
              >
                <option value="1">启用</option>
                <option value="0">停用</option>
              </Select>
            </Field>
          </div>
        )}
        {error && (
          <p role="alert" className="mt-3 text-sm text-error">
            {error}
          </p>
        )}
      </FormDialog>
    </>
  );
}
