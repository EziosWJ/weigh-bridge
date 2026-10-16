import { List, X } from "lucide-react";
import { useEffect, useId, useLayoutEffect, useRef, useState, type KeyboardEvent } from "react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Pagination } from "@/components/common/pagination";
import { Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogOverlay, DialogTitle } from "@/components/ui/dialog";
import type { ApiPageResult } from "@/types/api";
export type RecordInfo = {
    name: string;
    details: string[];
    tags: string[];
    code: string;
};
type Props<T extends {
    id: number;
}> = {
    label: string;
    inlineBrowse?: boolean;
    placeholder?: string;
    value: number;
    fallback?: string;
    disabled?: boolean;
    keyword?: string;
    onKeywordChange?: (value: string) => void;
    onQuery?: () => void;
    onDraftKeywordChange?: (value: string) => void;
    recentKey?: string;
    loadPage: (keyword: string, page: number, size: number) => Promise<ApiPageResult<T>>;
    loadRecord: (id: number) => Promise<T>;
    describe: (record: T) => RecordInfo;
    warning: (record: T) => string;
    onChange: (record: T | null) => void | boolean | Promise<void | boolean>;
};
function Highlight({ text, keyword }: {
    text: string;
    keyword: string;
}) {
    const words = keyword.trim().split(/\s+/).filter(Boolean).map(s => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"));
    if (!words.length)
        return <>{text}</>;
    const pattern = new RegExp(`(${words.join("|")})`, "gi");
    return <>{text.split(pattern).map((s, i) => i % 2 ? <mark key={i} className="bg-info-background font-semibold text-primary">{s}</mark> : s)}</>;
}
export function RecordPicker<T extends {
    id: number;
}>({ label, inlineBrowse = true, placeholder = "输入名称、型号或联系方式", value, fallback, disabled, keyword: applied = "", onKeywordChange, onQuery, onDraftKeywordChange, recentKey, loadPage, loadRecord, describe, warning, onChange }: Props<T>) {
    const id = useId(), input = useRef<HTMLInputElement>(null), panel = useRef<HTMLDivElement>(null);
    const [open, setOpen] = useState(false), [modal, setModal] = useState(false), [text, setText] = useState(applied), [page, setPage] = useState(1);
    const [records, setRecords] = useState<T[]>([]), [selected, setSelected] = useState<T | null>(null), [total, setTotal] = useState(0), [active, setActive] = useState(-1);
    const [loading, setLoading] = useState(false), [error, setError] = useState(""), [selectedError, setSelectedError] = useState(""), [retry, setRetry] = useState(0), [recent, setRecent] = useState(false);
    const change = useRef(onChange);
    change.current = onChange;
    const owner = useRef("");
    owner.current = `${recentKey}:${value}:${disabled}`;
    const generation = useRef(0), selecting = useRef(false), mounted = useRef(true), suppressFocus = useRef(false);
    useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
    useEffect(() => { setText(value > 0 ? "" : applied); }, [applied, value]);
    useEffect(() => { let live = true; setSelectedError(""); setSelected(null); if (value > 0)
        void loadRecord(value).then(r => { if (live)
            setSelected(r); }).catch(e => { if (live)
            setSelectedError(e instanceof Error ? e.message : "读取已选资料失败"); }); return () => { live = false; }; }, [value, loadRecord, retry, recentKey]);
    useEffect(() => {
        const current = ++generation.current;
        let live = true;
        if (!open && !modal)
            return;
        setLoading(true);
        setError("");
        setActive(-1);
        setRecords([]);
        setRecent(false);
        const timer = setTimeout(() => {
            void (async () => {
                if (!modal && !text.trim() && recentKey) {
                    let ids: number[] = [];
                    try {
                        const saved: unknown = JSON.parse(localStorage.getItem(recentKey) ?? "[]");
                        if (Array.isArray(saved))
                            ids = saved.filter((n): n is number => Number.isSafeInteger(n) && n > 0).slice(0, 24);
                    }
                    catch { /* Search remains usable without storage. */ }
                    if (ids.length) {
                        const results = await Promise.allSettled(ids.map(loadRecord));
                        const usable = results.flatMap(r => r.status === "fulfilled" && !warning(r.value) ? [r.value] : []).slice(0, 6);
                        if (usable.length)
                            return { records: usable, total: usable.length, isRecent: true };
                    }
                }
                const result = await loadPage(text.trim(), modal ? page : 1, modal ? 10 : 6);
                return { ...result, isRecent: false };
            })().then(r => { if (!live || current !== generation.current)
                return; setRecords(r.records); setTotal(r.total); setRecent(r.isRecent); }).catch(e => { if (live && current === generation.current)
                setError(e instanceof Error ? e.message : "搜索失败"); }).finally(() => { if (live && current === generation.current)
                setLoading(false); });
        }, 250);
        return () => { live = false; clearTimeout(timer); };
    }, [open, modal, text, page, retry, recentKey, loadPage, loadRecord, warning]);
    useLayoutEffect(() => {
        const p = panel.current, anchor = input.current;
        if (!open || modal || !p || !anchor)
            return;
        p.showPopover();
        const place = () => {
            const r = anchor.getBoundingClientRect(), v = window.visualViewport, w = v?.width ?? window.innerWidth, h = v?.height ?? window.innerHeight, left = v?.offsetLeft ?? 0, top = v?.offsetTop ?? 0;
            if (r.bottom < top || r.top > top + h) {
                setOpen(false);
                return;
            }
            p.style.width = `${Math.min(Math.max(r.width, 400), w - 16)}px`;
            p.style.maxHeight = `${Math.max(120, Math.min(420, h - 32))}px`;
            const ph = p.getBoundingClientRect().height, bottom = top + h - r.bottom - 8, above = r.top - top - 8;
            p.style.maxHeight = `${Math.max(0, Math.min(420, Math.max(bottom, above)))}px`;
            p.style.left = `${Math.max(left + 8, Math.min(r.left, left + w - p.getBoundingClientRect().width - 8))}px`;
            p.style.top = `${bottom < ph && above > bottom ? r.top - p.getBoundingClientRect().height - 4 : r.bottom + 4}px`;
        };
        place();
        const observer = new ResizeObserver(place);
        observer.observe(p);
        window.addEventListener("scroll", place, true);
        window.addEventListener("resize", place);
        return () => { observer.disconnect(); window.removeEventListener("scroll", place, true); window.removeEventListener("resize", place); if (p.matches(":popover-open"))
            p.hidePopover(); };
    }, [open, modal]);
    function close(restore = true) { ++generation.current; setOpen(false); setModal(false); setText(value > 0 ? "" : applied); onDraftKeywordChange?.(value > 0 ? "" : applied); setActive(-1); if (restore)
        requestAnimationFrame(() => { suppressFocus.current = true; input.current?.focus({ preventScroll: true }); suppressFocus.current = false; }); }
    async function choose(record: T | null) {
        if (selecting.current || disabled)
            return;
        selecting.current = true;
        ++generation.current;
        setOpen(false);
        setModal(false);
        setActive(-1);
        const selectionOwner = owner.current;
        try {
            const fresh = record ? await loadRecord(record.id) : null;
            if (!mounted.current || selectionOwner !== owner.current)
                return;
            if (fresh && warning(fresh)) {
                setSelectedError(warning(fresh));
                return;
            }
            const accepted = await change.current(fresh);
            if (!mounted.current)
                return;
            if (accepted === false) {
                setText(value > 0 ? "" : applied);
                requestAnimationFrame(() => input.current?.focus());
                return;
            }
            setSelectedError("");
            setSelected(fresh);
            setText("");
            if (fresh && recentKey) {
                try {
                    const raw: unknown = JSON.parse(localStorage.getItem(recentKey) ?? "[]");
                    const ids = Array.isArray(raw) ? raw.filter(n => Number.isSafeInteger(n) && n > 0) : [];
                    localStorage.setItem(recentKey, JSON.stringify([fresh.id, ...ids.filter(n => n !== fresh.id)].slice(0, 24)));
                }
                catch { /* Optional preference storage. */ }
            }
        }
        catch (e) {
            if (mounted.current)
                setSelectedError(e instanceof Error ? e.message : "读取资料失败，原选择已保留");
        }
        finally {
            selecting.current = false;
        }
    }
    function applyKeyword() { if (!onKeywordChange)
        return; const next = text.trim(); onKeywordChange(next); setOpen(false); setModal(false); setActive(-1); onQuery?.(); }
    function keys(e: KeyboardEvent<HTMLInputElement>) {
        if (e.nativeEvent.isComposing || e.keyCode === 229) {
            if (e.key === "Enter") {
                e.preventDefault();
                e.stopPropagation();
            }
            return;
        }
        if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            e.preventDefault();
            if (!modal)
                setOpen(true);
            setActive(a => records.length ? a < 0 ? (e.key === "ArrowDown" ? 0 : records.length - 1) : (a + (e.key === "ArrowDown" ? 1 : -1) + records.length) % records.length : -1);
        }
        if (e.key === "Escape") {
            e.preventDefault();
            e.stopPropagation();
            close();
        }
        if (e.key === "Enter") {
            e.preventDefault();
            e.stopPropagation();
            if (active >= 0 && records[active] && !loading && !warning(records[active]))
                void choose(records[active]);
            else if (!modal)
                applyKeyword();
        }
    }
    const current = selected?.id === value ? selected : null, info = current ? describe(current) : null;
    const search = (inModal = false) => <Input ref={inModal ? undefined : input} className={inlineBrowse && !inModal ? "min-w-0 w-full pr-20" : "min-w-0 grow basis-[12rem]"} autoFocus={inModal} role="combobox" aria-label={`搜索${label}`} aria-expanded={inModal || open} aria-haspopup={inModal ? "grid" : "listbox"} aria-controls={`${id}-${inModal ? "all" : "quick"}`} aria-activedescendant={active >= 0 ? `${id}-option-${active}` : undefined} disabled={disabled} value={inlineBrowse && !inModal && value > 0 && !text ? (info?.name ?? fallback ?? `#${value}`) : text} placeholder={placeholder} onFocus={() => { if (!inModal && !suppressFocus.current)
        setOpen(true); }} onChange={e => { setText(e.target.value); onDraftKeywordChange?.(e.target.value); setPage(1); setActive(-1); if (!inModal)
        setOpen(true); }} onKeyDown={keys}/>;
    const status = <>{loading && <p role="status" className="p-3 text-sm text-text-secondary">搜索中…</p>}{error && <div role="alert" className="p-3 text-sm text-error">{error} <Button onClick={() => setRetry(n => n + 1)}>重试搜索</Button></div>}{!loading && !error && !records.length && <p role="status" className="p-3 text-sm text-text-secondary">没有匹配档案{onKeywordChange ? "，仍可按关键词查询历史业务。" : "，请调整关键词。"}</p>}</>;
    function summary(r: T) { const d = describe(r); return <><strong className="block font-medium"><Highlight text={d.name} keyword={text}/></strong><span className="mt-1 flex flex-wrap gap-x-3 text-sm text-text-secondary">{d.details.map((s, i) => <span key={i}><Highlight text={s} keyword={text}/></span>)}</span></>; }
    return <div role="group" aria-label={label} className="min-w-0 space-y-2">
  <div className={inlineBrowse ? "relative min-w-0" : "flex min-w-0 flex-wrap gap-2"}>{search()}
    {inlineBrowse && (value > 0 || text || applied) && <Button type="button" variant="ghost" size="sm" className="absolute right-10 top-0 h-full px-2" aria-label={`清除${label}`} title={`清除${label}`} disabled={disabled} onClick={() => { setText(""); onDraftKeywordChange?.(""); if (value > 0) void choose(null); else if (onKeywordChange) onKeywordChange(""); }}><X className="h-4 w-4" aria-hidden /></Button>}
    <Button type="button" variant={inlineBrowse ? "ghost" : "primary"} className={inlineBrowse ? "absolute right-0 top-0 h-full px-3" : undefined} aria-label={inlineBrowse ? `浏览选择${label}` : undefined} title={inlineBrowse ? `浏览选择${label}` : undefined} disabled={disabled} onClick={() => { setOpen(false); setModal(true); setPage(1); setActive(-1); }}>{inlineBrowse ? <List className="h-4 w-4" aria-hidden /> : "浏览选择"}</Button></div>
  {!inlineBrowse && (value > 0 || applied) && <div className="flex min-w-0 items-center gap-2 text-sm"><span className="min-w-0 flex-1 truncate" title={info ? [info.name, ...info.details, ...info.tags, info.code].join("；") : fallback}>{value > 0 ? `指定${label}：${info?.name ?? fallback ?? `#${value}`}` : `${label}关键词：${applied}`}</span>{info && <details className="shrink-0"><summary className="cursor-pointer text-primary">资料</summary><p className="max-w-sm whitespace-normal text-text-secondary">{[...info.details, ...info.tags, info.code].join("；")}</p></details>}<Button disabled={disabled} size="sm" aria-label={`清除${label}`} onClick={() => { if (applied && onKeywordChange)
        onKeywordChange("");
    else
        void choose(null); }}>清除</Button></div>}
  {current && warning(current) && <p className="text-sm text-warning">{warning(current)}</p>}
  {selectedError && <div role="alert" className="text-sm text-error">{selectedError} <Button onClick={() => setRetry(n => n + 1)}>重试读取</Button></div>}
  {open && !modal && <div ref={panel} popover="auto" className="fixed inset-auto !m-0 overflow-auto rounded-control border border-border bg-surface text-text-primary shadow-floating" onToggle={e => { if (e.newState === "closed")
            setOpen(false); }} onMouseDown={e => e.preventDefault()}>
   {!text.trim() && <p className="px-3 py-2 text-xs text-text-tertiary">{recent ? "最近使用" : "暂无最近使用，以下为档案候选"}</p>}{status}
   <div id={`${id}-quick`} role="listbox" aria-label={`${label}候选`}>{!loading && !error && records.map((r, i) => { const d = describe(r); return <button key={r.id} type="button" id={`${id}-option-${i}`} role="option" aria-selected={active === i} disabled={Boolean(warning(r))} className={`block w-full border-b border-border px-3 py-2 text-left hover:bg-neutral-background ${active === i ? "bg-info-background" : ""}`} onClick={() => void choose(r)}>{summary(r)}<span className="mt-1 flex justify-between gap-2 text-xs text-text-tertiary"><span>{d.tags.join("，")}</span><span>{d.code}</span></span></button>; })}</div>
   {onKeywordChange && <Button className="m-2" onClick={applyKeyword}>按关键词查询</Button>}
   <Button variant="ghost" className="w-full" onClick={() => { setOpen(false); setModal(true); setPage(1); }}>{recent ? "浏览全部档案" : `查看全部结果${!loading && !error ? `（${total}）` : ""}`}</Button>
  </div>}
  <Dialog open={modal} onOpenChange={next => { if (!next)
        close(); }} trapFocus lockScroll>
   <DialogOverlay /><DialogContent className="flex max-w-[800px] flex-col" onKeyDown={e => { if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        close();
    } }}>
    <DialogHeader><DialogTitle>选择{label}</DialogTitle><Button variant="ghost" onClick={() => close()}>关闭</Button></DialogHeader>
    <DialogBody className="space-y-3">{search(true)}{status}<div className="overflow-x-auto"><table id={`${id}-all`} role="grid" className="w-full text-sm"><thead><tr className="border-b border-border text-left text-text-secondary"><th className="p-2">名称与辨认资料</th><th className="p-2">身份／类型与单位</th><th className="p-2">编码</th></tr></thead><tbody>{!loading && !error && records.map((r, i) => { const d = describe(r); return <tr key={r.id} id={`${id}-option-${i}`} aria-selected={active === i} onClick={() => { if (!warning(r)) void choose(r); }} className={`cursor-pointer border-b border-border hover:bg-neutral-background ${active === i ? "bg-info-background" : ""}`}><td className="p-2"><button type="button" disabled={Boolean(warning(r))} className="w-full text-left hover:text-primary" onClick={e => { e.stopPropagation(); void choose(r); }}>{summary(r)}</button></td><td className="p-2">{d.tags.join("，")}</td><td className="p-2 text-text-tertiary"><Highlight text={d.code} keyword={text}/></td></tr>; })}</tbody></table></div></DialogBody>
    <DialogFooter><Pagination page={page} pageSize={10} total={total} disabled={loading || Boolean(error)} onPageChange={setPage}/></DialogFooter>
   </DialogContent>
  </Dialog>
 </div>;
}
