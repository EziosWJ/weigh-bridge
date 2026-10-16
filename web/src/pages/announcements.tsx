import { useEffect, useState } from "react";
import { getAnnouncement, getAnnouncements, type Announcement } from "@/api/announcement";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { Button } from "@/components/ui/button";
import { Dialog, DialogBody, DialogContent, DialogHeader, DialogOverlay, DialogTitle } from "@/components/ui/dialog";
import { formatDateTime } from "@/lib/datetime";
import { useMessageEvents } from "@/lib/message-events";
import type { DataTableColumn } from "@/types";
import { isApiError } from "@/lib/api-error";

export function AnnouncementsPage() {
  const [scope, setScope] = useState<Announcement["scope"]>("PUBLIC");
  const [page, setPage] = useState(1);
  const [items, setItems] = useState<Announcement[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [refresh, setRefresh] = useState(0);
  const [selected, setSelected] = useState<Pick<Announcement, "id" | "scope"> | null>(null);
  const [detail, setDetail] = useState<Announcement | null>(null);
  const [invalid, setInvalid] = useState(false);
  const [detailError, setDetailError] = useState(false);
  const version = useMessageEvents((state) => scope === "PUBLIC" ? state.publicVersion : state.privateVersion);
  const ready = useMessageEvents((state) => scope === "PUBLIC" ? state.publicReady || state.publicStatus === "reconnecting" : state.privateReady || state.status === "reconnecting");
  useEffect(() => {
    if (!ready && refresh === 0) return;
    const controller = new AbortController(); setLoading(true);
    void getAnnouncements(scope, page, 10, controller.signal).then((result) => { if (!controller.signal.aborted) { setItems(result.records); setTotal(result.total); setError(false); } }).catch(() => { if (!controller.signal.aborted) setError(true); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [scope, page, version, ready, refresh]);
  useEffect(() => {
    if (!selected) return;
    const controller = new AbortController();
    void getAnnouncement(selected, controller.signal).then((item) => { if (!controller.signal.aborted) { setDetail(item); setInvalid(false); setDetailError(false); } }).catch((error) => { if (!controller.signal.aborted) { setDetail(null); setInvalid(isApiError(error) && error.type === "notFound"); setDetailError(!(isApiError(error) && error.type === "notFound")); } });
    return () => controller.abort();
  }, [selected, version]);
  useEffect(() => {
    const dates = items.map((item) => item.expiresAt ? Date.parse(item.expiresAt) : Infinity).filter(Number.isFinite);
    if (!dates.length) return;
    const timer = window.setTimeout(() => { setItems((current) => current.filter((item) => !item.expiresAt || Date.parse(item.expiresAt) > Date.now())); }, Math.min(Math.max(0, Math.min(...dates) - Date.now() + 10), 2147483647));
    return () => window.clearTimeout(timer);
  }, [items]);
  useEffect(() => {
    if (!detail?.expiresAt) return;
    const expiresAt = Date.parse(detail.expiresAt);
    let timer: number;
    const expire = () => {
      if (Date.now() < expiresAt) { timer = window.setTimeout(expire, Math.min(expiresAt - Date.now(), 2147483647)); return; }
      setDetail(null); setInvalid(true); setRefresh((value) => value + 1);
    };
    timer = window.setTimeout(expire, Math.min(Math.max(0, expiresAt - Date.now()), 2147483647));
    return () => window.clearTimeout(timer);
  }, [detail]);
  const columns: DataTableColumn<Announcement>[] = [
    { title: "标题", dataIndex: "title" },
    { title: "发布时间", key: "time", render: (_, item) => formatDateTime(item.publishTime ?? "") },
    { title: "到期时间", key: "expiry", render: (_, item) => item.expiresAt ? formatDateTime(item.expiresAt) : "长期有效" },
    { title: "操作", key: "actions", nowrap: true, align: "center", render: (_, item) => <Button variant="ghost" size="sm" onClick={() => { setDetail(null); setInvalid(false); setDetailError(false); setSelected({ id: item.id, scope: item.scope }); }}>查看</Button> },
  ];
  return <div><PageHeader title="公告" description="查看当前有效的公开公告和站内公告" actions={<Button variant="secondary" onClick={() => setRefresh((value) => value + 1)}>刷新</Button>} />
    <div className="mb-4 flex gap-2" role="group" aria-label="公告范围"><Button variant={scope === "PUBLIC" ? "primary" : "secondary"} onClick={() => { setScope("PUBLIC"); setPage(1); }}>公开公告</Button><Button variant={scope === "INTERNAL" ? "primary" : "secondary"} onClick={() => { setScope("INTERNAL"); setPage(1); }}>站内公告</Button></div>
    {error && <p role="alert" className="mb-4 text-sm text-error">公告加载失败，请点击刷新重试。</p>}
    <DataTableCard><DataTable columns={columns} dataSource={items} rowKey="id" loading={loading} /><Pagination page={page} pageSize={10} total={total} onPageChange={setPage} /></DataTableCard>
    {selected && <Dialog open onOpenChange={(open) => { if (!open) { setSelected(null); setDetail(null); } }}><DialogOverlay /><DialogContent><DialogHeader><DialogTitle>{detail?.title ?? (invalid ? "公告已失效" : detailError ? "公告加载失败" : "加载公告")}</DialogTitle></DialogHeader><DialogBody>{detail ? <p className="whitespace-pre-wrap text-sm leading-7">{detail.content}</p> : <p>{invalid ? "该公告已不可查看，请返回列表。" : detailError ? "暂时无法读取公告，请关闭后重试。" : "加载中…"}</p>}<Button variant="secondary" className="mt-4" onClick={() => { setSelected(null); setDetail(null); }}>关闭</Button></DialogBody></DialogContent></Dialog>}
  </div>;
}
