import { CheckCheck, Eye, RefreshCw } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { getNotification, getNotifications, markAllNotificationsRead } from "@/api/notification";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogHeader, DialogOverlay, DialogTitle } from "@/components/ui/dialog";
import { formatDateTime } from "@/lib/datetime";
import { getErrorMessage } from "@/lib/api-error";
import type { DataTableColumn, NotificationRecord } from "@/types";
import { safeMessagePath, useMessageEvents } from "@/lib/message-events";

export function NotificationsPage() {
  const navigate = useNavigate();
  const { id } = useParams();
  const [records, setRecords] = useState<NotificationRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [detail, setDetail] = useState<NotificationRecord | null>(null);
  const ready = useMessageEvents((state) => state.privateReady || state.status === "reconnecting");
  const version = useMessageEvents((state) => state.notificationVersion);
  const status = useMessageEvents((state) => state.status);
  const load = useCallback(async (signal?: AbortSignal) => { setLoading(true); try { const result = await getNotifications({ page, pageSize: 10 }, signal); if (!signal?.aborted) { setRecords(result.records); setTotal(result.total); } } catch (error) { if (!signal?.aborted) toast.error({ title: "通知加载失败", description: getErrorMessage(error, "请稍后重试") }); } finally { if (!signal?.aborted) setLoading(false); } }, [page]);
  useEffect(() => { if (!ready) return; const controller = new AbortController(); void load(controller.signal); return () => controller.abort(); }, [load, ready, version]);
  useEffect(() => { if (!id) { setDetail(null); return; } const controller = new AbortController(); void getNotification(Number(id), controller.signal).then((value) => { if (!controller.signal.aborted) { setDetail(value); setRecords((current) => current.map((item) => item.id === value.id ? { ...item, isRead: 1 } : item)); } }).catch((error) => { if (!controller.signal.aborted) { toast.error({ title: "通知详情加载失败", description: getErrorMessage(error, "请稍后重试") }); navigate("/notifications", { replace: true }); } }); return () => controller.abort(); }, [id, navigate]);
  const readAll = async () => { try { await markAllNotificationsRead(); setRecords((current) => current.map((item) => ({ ...item, isRead: 1 }))); toast.success("已全部标记为已读"); } catch (error) { toast.error({ title: "操作失败", description: getErrorMessage(error, "请稍后重试") }); } };
  const columns: DataTableColumn<NotificationRecord>[] = [
    { title: "标题", key: "title", render: (_, item) => <div className={item.isRead ? "text-text-secondary" : "font-semibold text-text-primary"}>{item.title}</div> },
    { title: "来源", dataIndex: "sourceType", width: 120, render: (value) => value === "ROLE_CHANGE" ? "角色变更" : value === "BUSINESS" ? "业务通知" : "管理员发布" },
    { title: "时间", dataIndex: "publishTime", width: 180, render: (value) => formatDateTime(String(value)) },
    { title: "状态", dataIndex: "isRead", width: 90, render: (value) => value ? "已读" : "未读" },
    { title: "操作", key: "actions", align: "center", nowrap: true, width: 120, render: (_, item) => <Button size="sm" variant="ghost" onClick={() => navigate(`/notifications/${item.id}`)}><Eye className="h-4 w-4" />查看</Button> },
  ];
  return (
    <div>
      <PageHeader title="我的通知" description="查看发给你的站内通知" actions={<><Button variant="secondary" size="sm" onClick={() => void load()}><RefreshCw className="h-4 w-4" />刷新</Button><Button variant="secondary" size="sm" onClick={() => void readAll()}><CheckCheck className="h-4 w-4" />全部已读</Button></>} />
      {status !== "connected" && <p role="status" className="mb-3 text-sm text-text-tertiary">{status === "reconnecting" ? "消息连接中断，正在重新连接；也可手动刷新。" : "正在同步消息…"}</p>}
      <DataTableCard className="overflow-hidden shadow-none">
        <DataTable columns={columns} dataSource={records} rowKey="id" loading={loading} empty={<EmptyState title="暂无通知" description="你还没有收到站内通知。" />} />
        <Pagination page={page} pageSize={10} total={total} onPageChange={setPage} />
      </DataTableCard>
      {detail && (
        <Dialog
          open
          onOpenChange={(nextOpen) => { if (!nextOpen) navigate("/notifications"); }}
          closeOnEscape={false}
          closeOnOverlayClick={false}
        >
          <DialogOverlay />
          <DialogContent className="max-w-2xl p-6">
            <DialogHeader className="gap-4 border-0 p-0">
              <div>
                <DialogTitle className="text-xl">{detail.title}</DialogTitle>
                <DialogDescription className="mt-1 text-xs">
                  {formatDateTime(detail.publishTime)} · {detail.sourceType === "ROLE_CHANGE" ? "角色变更" : detail.sourceType === "BUSINESS" ? "业务通知" : "管理员发布"}
                </DialogDescription>
              </div>
              <Button variant="ghost" onClick={() => navigate("/notifications")}>关闭</Button>
            </DialogHeader>
            <DialogBody className="mt-6 max-h-none overflow-visible p-0">
              <p className="whitespace-pre-wrap text-sm leading-7 text-text-secondary">{detail.content}</p>
              {safeMessagePath(detail.jumpPath) && <Button variant="primary" className="mt-4" onClick={() => navigate(safeMessagePath(detail.jumpPath)!)}>前往业务页面</Button>}
            </DialogBody>
          </DialogContent>
        </Dialog>
      )}
    </div>
  );
}
