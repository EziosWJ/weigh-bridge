import { useCallback, useEffect, useState, type FormEvent } from "react";
import { getAdminAnnouncements, publishAnnouncement, saveAnnouncement, withdrawAnnouncement, type Announcement, type AnnouncementInput } from "@/api/announcement";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { Field } from "@/components/common/field";
import { FormSection } from "@/components/common/form-section";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { TableActions } from "@/components/common/table-actions";
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
  const [expiry, setExpiry] = useState("");
  const [items, setItems] = useState<Announcement[]>([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [detail, setDetail] = useState<Announcement | null>(null);
  const load = useCallback(async () => {
    setLoading(true);
    try { const result = await getAdminAnnouncements(page); setItems(result.records); setTotal(result.total); }
    catch (error) { toast.error({ title: "公告加载失败", description: getErrorMessage(error, "请稍后重试") }); }
    finally { setLoading(false); }
  }, [page]);
  useEffect(() => { void load(); }, [load]);
  const reset = () => { setForm(empty); setEditing(undefined); setExpiry(""); };
  const save = async (event: FormEvent) => {
    event.preventDefault();
    const date = expiry ? new Date(expiry) : null;
    if (date && (!Number.isFinite(date.getTime()) || date.getTime() <= Date.now())) { toast.error("到期时间必须晚于当前时间"); return; }
    setBusy(true);
    try { await saveAnnouncement({ ...form, expiresAt: date?.toISOString() ?? null }, editing); reset(); toast.success("草稿已保存"); await load(); }
    catch (error) { toast.error({ title: "保存失败", description: getErrorMessage(error, "请稍后重试") }); }
    finally { setBusy(false); }
  };
  const transition = async (item: Announcement, publish: boolean) => {
    if (!window.confirm(publish ? `确认发布“${item.title}”？${item.scope === "PUBLIC" ? "未登录访问者也能查看。" : "仅登录用户可查看。"}发布后内容不可修改。` : `确认撤下“${item.title}”？`)) return;
    setBusy(true);
    try { if (publish) await publishAnnouncement(item.id); else await withdrawAnnouncement(item.id); toast.success(publish ? "公告已发布" : "公告已撤下"); await load(); }
    catch (error) { toast.error({ title: "操作失败", description: getErrorMessage(error, "请稍后重试") }); }
    finally { setBusy(false); }
  };
  const edit = (item: Announcement) => {
    setEditing(item.id); setForm({ title: item.title, content: item.content, scope: item.scope, expiresAt: item.expiresAt });
    if (item.expiresAt) { const date = new Date(item.expiresAt); setExpiry(new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16)); } else setExpiry("");
    window.scrollTo({ top: 0, behavior: "instant" });
  };
  const columns: DataTableColumn<Announcement>[] = [
    { title: "标题", dataIndex: "title" },
    { title: "范围", dataIndex: "scope", render: (value) => value === "PUBLIC" ? "公开公告" : "站内公告" },
    { title: "状态", key: "status", render: (_, item) => item.status === "DRAFT" ? "草稿" : item.status === "WITHDRAWN" ? "已撤下" : item.expiresAt && Date.parse(item.expiresAt) <= Date.now() ? "已到期" : "已发布" },
    { title: "发布时间", key: "time", render: (_, item) => item.publishTime ? formatDateTime(item.publishTime) : "—" },
    { title: "到期时间", key: "expiry", render: (_, item) => item.expiresAt ? formatDateTime(item.expiresAt) : "长期有效" },
    { title: "操作", key: "actions", nowrap: true, align: "center", render: (_, item) => <TableActions><Button variant="ghost" size="sm" onClick={() => setDetail(item)}>查看</Button>{item.status === "DRAFT" ? <><Button variant="ghost" size="sm" disabled={busy} onClick={() => edit(item)}>编辑</Button><Button variant="ghost" size="sm" disabled={busy} onClick={() => void transition(item, true)}>发布</Button></> : item.status === "PUBLISHED" ? <Button variant="ghost" size="sm" disabled={busy} onClick={() => void transition(item, false)}>撤下</Button> : null}</TableActions> },
  ];
  return <div className="space-y-6"><PageHeader title="公告管理" description="公开公告无需登录即可查看，发布前请确认内容适合公开" actions={<Button variant="secondary" onClick={() => void load()}>刷新</Button>} />
    <form onSubmit={(event) => void save(event)}><FormSection title={editing ? "编辑公告草稿" : "新建公告草稿"}>
      <Field label="标题" htmlFor="announcement-title" required><Input id="announcement-title" value={form.title} maxLength={200} required onChange={(event) => setForm({ ...form, title: event.target.value })} /></Field>
      <Field label="可见范围" htmlFor="announcement-scope"><select id="announcement-scope" className="h-9 w-full rounded-md border border-border bg-surface px-3 text-sm" value={form.scope} onChange={(event) => setForm({ ...form, scope: event.target.value as Announcement["scope"] })}><option value="INTERNAL">站内公告（登录后可见）</option><option value="PUBLIC">公开公告（无需登录）</option></select></Field>
      <div className="md:col-span-2"><Field label="正文" htmlFor="announcement-content" required><Textarea id="announcement-content" rows={5} maxLength={5000} required value={form.content} onChange={(event) => setForm({ ...form, content: event.target.value })} /></Field></div>
      <Field label="到期时间" htmlFor="announcement-expiry" help="留空表示长期有效；填写本地时间"><Input id="announcement-expiry" type="datetime-local" value={expiry} onChange={(event) => setExpiry(event.target.value)} /></Field>
      <div className="flex items-end gap-2 pb-6"><Button type="submit" variant="primary" disabled={busy}>{busy ? "保存中…" : "保存草稿"}</Button><Button type="button" variant="secondary" disabled={busy} onClick={reset}>取消编辑</Button></div>
    </FormSection></form>
    <DataTableCard><DataTable columns={columns} dataSource={items} rowKey="id" loading={loading} /><Pagination page={page} pageSize={10} total={total} onPageChange={setPage} /></DataTableCard>
    {detail && <Dialog open onOpenChange={(open) => { if (!open) setDetail(null); }}><DialogOverlay /><DialogContent><DialogHeader><DialogTitle>{detail.title}</DialogTitle></DialogHeader><DialogBody><p className="whitespace-pre-wrap text-sm leading-7">{detail.content}</p><Button variant="secondary" className="mt-4" onClick={() => setDetail(null)}>关闭</Button></DialogBody></DialogContent></Dialog>}
  </div>;
}
