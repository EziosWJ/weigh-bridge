import { useCallback, useEffect, useState } from "react";
import { CalendarDays, ChevronLeft, ChevronRight, Megaphone, X } from "lucide-react";
import { Link } from "react-router-dom";
import { getAnnouncement, getAnnouncements, type Announcement } from "@/api/announcement";
import { Button } from "@/components/ui/button";
import { Dialog, DialogBody, DialogContent, DialogHeader, DialogOverlay, DialogTitle } from "@/components/ui/dialog";
import { useMessageEvents } from "@/lib/message-events";
import { useAuthStore } from "@/store/auth-store";
import { isApiError } from "@/lib/api-error";

export function AnnouncementPanel({ authenticated = false }: { authenticated?: boolean }) {
  const publicVersion = useMessageEvents((state) => state.publicVersion);
  const privateVersion = useMessageEvents((state) => state.privateVersion);
  const publicReady = useMessageEvents((state) => state.publicReady || state.publicStatus === "reconnecting");
  const privateReady = useMessageEvents((state) => state.privateReady || state.status === "reconnecting");
  const token = useAuthStore((state) => state.token);
  const [items, setItems] = useState<Announcement[]>([]);
  const dismissed = useMessageEvents((state) => state.dismissedAnnouncements);
  const [detail, setDetail] = useState<Announcement | null>(null);
  const [invalid, setInvalid] = useState(false);
  const [detailError, setDetailError] = useState(false);
  const [error, setError] = useState(false);
  const [list, setList] = useState(false);
  const detailID = detail?.id;
  const detailScope = detail?.scope;
  const load = useCallback(async (signal?: AbortSignal) => {
    if (!publicReady) return;
    try {
      const pages = await Promise.all([getAnnouncements("PUBLIC", 1, 5, signal), ...(authenticated && token && privateReady ? [getAnnouncements("INTERNAL", 1, 5, signal)] : [])]);
      if (signal?.aborted) return;
      setItems(pages.flatMap((page) => page.records).sort((a, b) => b.id - a.id)); setError(false);
    } catch { if (!signal?.aborted) setError(true); }
  }, [authenticated, token, publicReady, privateReady]);
  useEffect(() => { const controller = new AbortController(); void load(controller.signal); return () => controller.abort(); }, [load, publicVersion, privateVersion]);
  const open = async (item: Announcement) => {
    try { setDetail(await getAnnouncement(item)); setInvalid(false); setDetailError(false); } catch (error) { setDetail(null); setInvalid(isApiError(error) && error.type === "notFound"); setDetailError(!(isApiError(error) && error.type === "notFound")); }
  };
  useEffect(() => {
    if (!detailID || !detailScope) return;
    const controller = new AbortController();
    void getAnnouncement({ id: detailID, scope: detailScope }, controller.signal).then((next) => { if (!controller.signal.aborted) setDetail(next); }).catch((error) => { if (!controller.signal.aborted) { setDetail(null); setInvalid(isApiError(error) && error.type === "notFound"); setDetailError(!(isApiError(error) && error.type === "notFound")); } });
    return () => controller.abort();
  }, [detailID, detailScope, publicVersion, privateVersion]);
  useEffect(() => {
    const dates = [...items, ...(detail ? [detail] : [])].map((item) => item.expiresAt ? Date.parse(item.expiresAt) : Infinity).filter(Number.isFinite);
    if (!dates.length) return;
    const timer = window.setTimeout(() => { setItems((current) => current.filter((item) => !item.expiresAt || Date.parse(item.expiresAt) > Date.now())); if (detail?.expiresAt && Date.parse(detail.expiresAt) <= Date.now()) { setDetail(null); setInvalid(true); } }, Math.min(Math.max(0, Math.min(...dates) - Date.now() + 10), 2147483647));
    return () => window.clearTimeout(timer);
  }, [items, detail]);
  const visible = items.filter((item) => !dismissed.has(item.id) && (!item.expiresAt || Date.parse(item.expiresAt) > Date.now()));
  return <>
    {(visible.length > 0 || error) && <section aria-label="公告栏" className="mb-4 rounded-admin border border-border bg-surface p-3 text-sm">
      <div className="flex flex-wrap items-center gap-3"><Megaphone className="h-4 w-4 text-primary" aria-hidden /><span className="font-medium">公告</span>
        {visible.map((item) => <span key={item.id} className="inline-flex max-w-full items-center gap-1"><button type="button" className="truncate text-primary hover:underline" onClick={() => void open(item)}>{item.title}</button><button type="button" aria-label={`关闭公告：${item.title}`} className="rounded p-1 text-text-tertiary hover:bg-background" onClick={() => useMessageEvents.setState((state) => ({ dismissedAnnouncements: new Set(state.dismissedAnnouncements).add(item.id) }))}><X className="h-3.5 w-3.5" /></button></span>)}
        {error && <button type="button" onClick={() => void load()} className="text-text-secondary">公告加载失败，点击重试</button>}
        {authenticated ? <Link to="/announcements" className="ml-auto text-primary hover:underline">查看公告</Link> : <button type="button" onClick={() => setList(true)} className="ml-auto text-primary hover:underline">查看公告</button>}
      </div>
    </section>}
    {!authenticated && visible.length === 0 && <button type="button" onClick={() => setList(true)} className="mb-6 flex w-full items-center gap-3 rounded-admin border border-info-border bg-info-background px-4 py-3 text-left transition-colors hover:border-primary/30 hover:bg-primary/[0.04] focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary">
      <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-control bg-white text-primary shadow-sm"><Megaphone className="h-4 w-4" aria-hidden /></span>
      <span className="min-w-0 flex-1"><span className="block text-sm font-medium text-text-primary">查看公告</span><span className="mt-0.5 block text-xs text-text-tertiary">了解平台通知与维护安排</span></span>
      <ChevronRight className="h-4 w-4 shrink-0 text-text-tertiary" aria-hidden />
    </button>}
    {(detail || invalid || detailError || list) && <Dialog open onOpenChange={(next) => { if (!next) { setDetail(null); setInvalid(false); setDetailError(false); setList(false); } }}>
      <DialogOverlay /><DialogContent className="max-w-[560px]">
        <DialogHeader className="items-center px-5 py-4 sm:px-6"><div className="flex min-w-0 items-center gap-3"><span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-admin bg-info-background text-primary"><Megaphone className="h-5 w-5" aria-hidden /></span><div className="min-w-0"><DialogTitle className="truncate">{detail?.title ?? (invalid ? "公告已失效" : detailError ? "公告加载失败" : "公开公告")}</DialogTitle><p className="mt-1 text-xs text-text-tertiary">{detail ? formatDate(detail.publishTime) : "面向所有访客发布的平台信息"}</p></div></div><button type="button" aria-label="关闭" onClick={() => { setDetail(null); setInvalid(false); setDetailError(false); setList(false); }} className="rounded-control p-2 text-text-tertiary hover:bg-neutral-background hover:text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary"><X className="h-4 w-4" aria-hidden /></button></DialogHeader>
        <DialogBody className="max-h-[min(60vh,480px)] overflow-y-auto px-5 py-5 sm:px-6">
          {detail ? <p className="whitespace-pre-wrap text-sm leading-7 text-text-secondary">{detail.content}</p> : invalid ? <p className="rounded-admin bg-neutral-background px-4 py-5 text-sm text-text-secondary">该公告已不可查看，请返回公告列表。</p> : detailError ? <p className="rounded-admin bg-error-background px-4 py-5 text-sm text-error">暂时无法读取公告，请关闭后重新打开。</p> : <PublicAnnouncementList onOpen={(item) => { setList(false); void open(item); }} />}
        </DialogBody>
        <footer className="flex items-center justify-between border-t border-border px-5 py-3 sm:px-6"><span className="text-xs text-text-tertiary">{detail ? "公开公告" : "公告内容以当前有效信息为准"}</span><Button variant="secondary" size="sm" onClick={() => { setDetail(null); setInvalid(false); setDetailError(false); setList(false); }}>关闭</Button></footer>
      </DialogContent>
    </Dialog>}
  </>;
}

function formatDate(value?: string | null) {
  if (!value) return "平台公告";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "平台公告" : new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium" }).format(date);
}

function PublicAnnouncementList({ onOpen }: { onOpen: (item: Announcement) => void }) {
  const [items, setItems] = useState<Announcement[]>([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [error, setError] = useState(false);
  const version = useMessageEvents((state) => state.publicVersion);
  useEffect(() => { const controller = new AbortController(); void getAnnouncements("PUBLIC", page, 10, controller.signal).then((result) => { if (!controller.signal.aborted) { setItems(result.records); setTotal(result.total); setError(false); } }).catch(() => { if (!controller.signal.aborted) setError(true); }); return () => controller.abort(); }, [page, version]);
  return <div>{error ? <p className="rounded-admin bg-error-background px-4 py-5 text-sm text-error">公告加载失败，请关闭后重试。</p> : items.length === 0 ? <p className="rounded-admin bg-neutral-background px-4 py-8 text-center text-sm text-text-tertiary">暂无有效公告</p> : <div className="divide-y divide-border overflow-hidden rounded-admin border border-border">{items.map((item) => <button key={item.id} type="button" className="group flex w-full items-start gap-3 px-4 py-4 text-left transition-colors hover:bg-neutral-background focus-visible:outline focus-visible:outline-2 focus-visible:outline-inset focus-visible:outline-primary" onClick={() => onOpen(item)}><span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-control bg-info-background text-primary"><Megaphone className="h-4 w-4" aria-hidden /></span><span className="min-w-0 flex-1"><span className="block truncate text-sm font-medium text-text-primary group-hover:text-primary">{item.title}</span><span className="mt-1 line-clamp-2 block text-xs leading-5 text-text-tertiary">{item.content}</span><span className="mt-2 inline-flex items-center gap-1 text-[11px] text-text-tertiary"><CalendarDays className="h-3 w-3" aria-hidden />{formatDate(item.publishTime)}</span></span><ChevronRight className="mt-2 h-4 w-4 shrink-0 text-text-tertiary group-hover:text-primary" aria-hidden /></button>)}</div>}
    <div className="mt-4 flex items-center justify-between"><span className="text-xs text-text-tertiary">共 {total} 条公告</span><div className="flex items-center gap-2"><Button variant="secondary" size="sm" disabled={page === 1} aria-label="上一页" onClick={() => setPage(page - 1)}><ChevronLeft className="h-4 w-4" aria-hidden /></Button><span className="min-w-14 text-center text-xs tabular-nums text-text-secondary">{page} / {Math.max(1, Math.ceil(total / 10))}</span><Button variant="secondary" size="sm" disabled={page * 10 >= total} aria-label="下一页" onClick={() => setPage(page + 1)}><ChevronRight className="h-4 w-4" aria-hidden /></Button></div></div>
  </div>;
}
