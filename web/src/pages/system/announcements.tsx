import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { getAdminAnnouncements, publishAnnouncement, saveAnnouncement, withdrawAnnouncement, type Announcement, type AnnouncementInput } from "@/api/announcement";
import { ConfirmDialog } from "@/components/common/confirm-dialog";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { EmptyState } from "@/components/common/empty-state";
import { Field } from "@/components/common/field";
import { FormSection } from "@/components/common/form-section";
import { PageHeader } from "@/components/common/page-header";
import { StatusTag } from "@/components/common/status-tag";
import { Pagination } from "@/components/common/pagination";
import { TableActionMenu, TableActions } from "@/components/common/table-actions";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Dialog, DialogBody, DialogContent, DialogHeader, DialogOverlay, DialogTitle } from "@/components/ui/dialog";
import { getErrorMessage } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import type { DataTableColumn } from "@/types";

const empty: AnnouncementInput = { title: "", content: "", scope: "INTERNAL", expiresAt: null };
export function AnnouncementManagePage() {
  const [form, setForm] = useState<AnnouncementInput>(empty);
  const [editing, setEditing] = useState<number>();
  const titleInput = useRef<HTMLInputElement>(null);
  const [expiry, setExpiry] = useState("");
  const [items, setItems] = useState<Announcement[]>([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [detail, setDetail] = useState<Announcement | null>(null);
  const [pending, setPending] = useState<{ item: Announcement; publish: boolean } | null>(null);
  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try { const result = await getAdminAnnouncements(page); setItems(result.records); setTotal(result.total); }
    catch (error) { setError(getErrorMessage(error, "无法读取公告列表")); }
    finally { setLoading(false); }
  }, [page]);
  useEffect(() => { void load(); }, [load]);
  const reset = () => { setForm(empty); setEditing(undefined); setExpiry(""); };
  const save = async (event: FormEvent) => {
    event.preventDefault();
    const date = expiry ? new Date(`${expiry.length === 16 ? `${expiry}:00` : expiry}+08:00`) : null;
    if (date && (!Number.isFinite(date.getTime()) || date.getTime() <= Date.now())) { toast.error("到期时间必须晚于当前时间"); return; }
    setBusy(true);
    try { await saveAnnouncement({ ...form, expiresAt: date?.toISOString() ?? null }, editing); reset(); toast.success("草稿已保存"); await load(); }
    catch (error) { toast.error({ title: "保存失败", description: getErrorMessage(error, "请稍后重试") }); }
    finally { setBusy(false); }
  };
  const transition = async () => {
    if (!pending) return;
    const { item, publish } = pending;
    setBusy(true);
    try {
      if (publish) await publishAnnouncement(item.id);
      else await withdrawAnnouncement(item.id);
      setPending(null);
      toast.success(publish ? "公告已发布" : "公告已撤下");
      await load();
    } catch (error) {
      toast.error({ title: publish ? "发布失败" : "撤下失败", description: getErrorMessage(error, "请稍后重试") });
    } finally {
      setBusy(false);
    }
  };
  const edit = (item: Announcement) => {
    setEditing(item.id); setForm({ title: item.title, content: item.content, scope: item.scope, expiresAt: item.expiresAt });
    if (item.expiresAt) setExpiry(formatDateTime(item.expiresAt).replace(" ", "T").slice(0, 16)); else setExpiry("");
    titleInput.current?.focus();
  };
  const columns: DataTableColumn<Announcement>[] = [
    { title: "标题", dataIndex: "title" },
    { title: "范围", dataIndex: "scope", width: 104, render: (value) => value === "PUBLIC" ? "公开公告" : "站内公告" },
    {
      title: "状态",
      key: "status",
      width: 88,
      render: (_, item) => {
        if (item.status === "DRAFT") return <StatusTag tone="neutral">草稿</StatusTag>;
        if (item.status === "WITHDRAWN") return <StatusTag tone="neutral">已撤下</StatusTag>;
        if (item.expiresAt && Date.parse(item.expiresAt) <= Date.now()) return <StatusTag tone="neutral">已到期</StatusTag>;
        return <StatusTag tone="success">已发布</StatusTag>;
      },
    },
    { title: "发布时间", key: "time", width: 164, nowrap: true, render: (_, item) => item.publishTime ? formatDateTime(item.publishTime) : "—" },
    { title: "到期时间", key: "expiry", width: 164, nowrap: true, render: (_, item) => item.expiresAt ? formatDateTime(item.expiresAt) : "长期有效" },
    {
      title: "操作",
      key: "actions",
      nowrap: true,
      align: "center",
      width: 200,
      render: (_, item) => (
        <TableActions>
          <Button variant="secondary" size="sm" onClick={() => setDetail(item)}>详情</Button>
          {item.status === "DRAFT" && <Button variant="ghost" size="sm" disabled={busy} onClick={() => edit(item)}>编辑</Button>}
          <TableActionMenu items={
            item.status === "DRAFT"
              ? [{ label: "发布", disabled: busy, onSelect: () => setPending({ item, publish: true }) }]
              : item.status === "PUBLISHED"
                ? [{ label: "撤下", disabled: busy, danger: true, onSelect: () => setPending({ item, publish: false }) }]
                : []
          } />
        </TableActions>
      ),
    },
  ];
  return <div className="space-y-6"><PageHeader title="公告管理" description="" actions={<Button variant="secondary" disabled={loading} onClick={() => void load()}>刷新</Button>} />
    <form onSubmit={(event) => void save(event)}><FormSection title={editing ? "编辑公告草稿" : "新建公告草稿"}>
      <Field label="标题" htmlFor="announcement-title" required><Input ref={titleInput} id="announcement-title" value={form.title} maxLength={200} required onChange={(event) => setForm({ ...form, title: event.target.value })} /></Field>
      <Field label="可见范围" htmlFor="announcement-scope"><select id="announcement-scope" className="h-9 w-full rounded-md border border-border bg-surface px-3 text-sm" value={form.scope} onChange={(event) => setForm({ ...form, scope: event.target.value as Announcement["scope"] })}><option value="INTERNAL">站内公告（登录后可见）</option><option value="PUBLIC">公开公告（无需登录）</option></select></Field>
      <div className="md:col-span-2"><Field label="正文" htmlFor="announcement-content" required><Textarea id="announcement-content" rows={5} maxLength={5000} required value={form.content} onChange={(event) => setForm({ ...form, content: event.target.value })} /></Field></div>
      <Field label="到期时间" htmlFor="announcement-expiry" help="留空表示长期有效；按北京时间填写"><Input id="announcement-expiry" type="datetime-local" value={expiry} onChange={(event) => setExpiry(event.target.value)} /></Field>
      <div className="flex items-end gap-2 pb-6"><Button type="submit" variant="primary" disabled={busy}>{busy ? "保存中…" : "保存草稿"}</Button><Button type="button" variant="secondary" disabled={busy} onClick={reset}>取消编辑</Button></div>
    </FormSection></form>
    <DataTableCard>
      <DataTable
        columns={columns}
        dataSource={items}
        rowKey="id"
        loading={loading}
        error={error && <EmptyState title="公告加载失败" description={error} actionText="重试" onAction={() => void load()} />}
        empty={<EmptyState title="暂无公告" description="保存的草稿和已发布的公告会显示在这里。" />}
        minWidth={840}
        compact
        fixedActions
        ariaLabel="公告列表"
      />
      <Pagination page={page} pageSize={10} total={total} disabled={loading || Boolean(error)} onPageChange={setPage} />
    </DataTableCard>
    <ConfirmDialog
      open={pending !== null}
      title={pending?.publish ? "发布公告" : "撤下公告"}
      description={pending?.publish
        ? `公告「${pending.item.title}」发布后内容不可修改。${pending.item.scope === "PUBLIC" ? "访客也能查看。" : "登录用户可查看。"}`
        : pending && `撤下公告「${pending.item.title}」后，阅读者将无法继续查看。`}
      confirmText={pending?.publish ? "发布" : "撤下"}
      danger={pending?.publish === false}
      loading={busy}
      onConfirm={() => void transition()}
      onCancel={() => setPending(null)}
    />
    {detail && <Dialog open onOpenChange={(open) => { if (!open) setDetail(null); }}><DialogOverlay /><DialogContent><DialogHeader><DialogTitle>{detail.title}</DialogTitle></DialogHeader><DialogBody><p className="whitespace-pre-wrap text-sm leading-7">{detail.content}</p><Button variant="secondary" className="mt-4" onClick={() => setDetail(null)}>关闭</Button></DialogBody></DialogContent></Dialog>}
  </div>;
}
