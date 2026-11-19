import { Send, RefreshCw } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { getAdminNotifications, publishNotification } from "@/api/notification";
import { getUserPage } from "@/api/user";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/common/page-header";
import { TableToolbar } from "@/components/common/table-toolbar";
import { Pagination } from "@/components/common/pagination";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { getErrorMessage } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import type { DataTableColumn, NotificationRecord, UserRecord } from "@/types";

export function NotificationManagePage() {
  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");
  const [users, setUsers] = useState<UserRecord[]>([]);
  const [selected, setSelected] = useState<number[]>([]);
  const [all, setAll] = useState(true);
  const [records, setRecords] = useState<NotificationRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [publishing, setPublishing] = useState(false);
  const [historyError, setHistoryError] = useState("");
  const load = useCallback(async () => {
    setLoading(true);
    setHistoryError("");
    const [people, history] = await Promise.allSettled([
      getUserPage({ page: 1, pageSize: 500, status: 1 }),
      getAdminNotifications({ page, pageSize: 10 }),
    ]);
    if (people.status === "fulfilled") {
      setUsers(people.value.records);
    } else {
      toast.error({ title: "接收用户加载失败", description: getErrorMessage(people.reason, "请稍后重试") });
    }
    if (history.status === "fulfilled") {
      setRecords(history.value.records);
      setTotal(history.value.total);
    } else {
      setHistoryError(getErrorMessage(history.reason, "无法读取通知历史"));
    }
    setLoading(false);
  }, [page]);
  useEffect(() => { void load(); }, [load]);
  const publish = async () => {
    if (!title.trim() || !content.trim()) {
      toast.error("请填写标题和正文");
      return;
    }
    if (!all && selected.length === 0) {
      toast.error("请选择接收用户");
      return;
    }
    setPublishing(true);
    try {
      await publishNotification({ title, content, userIds: all ? [] : selected, allUsers: all });
      setTitle("");
      setContent("");
      setSelected([]);
      toast.success("通知已发布");
      await load();
    } catch (error) {
      toast.error({ title: "发布失败", description: getErrorMessage(error, "请稍后重试") });
    } finally {
      setPublishing(false);
    }
  };
  const columns: DataTableColumn<NotificationRecord>[] = [
    { title: "标题", dataIndex: "title" },
    {
      title: "来源",
      dataIndex: "sourceType",
      width: 116,
      render: (value) => value === "ROLE_CHANGE" ? "角色变更" : "管理员发布",
    },
    {
      title: "已读 / 接收",
      key: "read",
      align: "right",
      width: 112,
      render: (_, item) => <span className="tabular-nums">{item.readCount ?? 0} / {item.recipientCount ?? 0}</span>,
    },
    {
      title: "发布时间",
      dataIndex: "publishTime",
      width: 172,
      nowrap: true,
      render: (value) => <span className="tabular-nums">{formatDateTime(String(value ?? ""))}</span>,
    },
  ];
  return (
    <div>
      <PageHeader
        title="通知管理"
        description=""
        actions={
          <Button variant="secondary" size="sm" disabled={loading} onClick={() => void load()}>
            <RefreshCw className="h-4 w-4" aria-hidden />刷新
          </Button>
        }
      />
      <div className="mb-6 rounded-admin border border-border bg-surface p-5">
        <h2 className="mb-4 text-base font-semibold text-text-primary">发布通知</h2>
        <div className="grid gap-4">
          <Input
            aria-label="通知标题"
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            placeholder="通知标题"
            maxLength={200}
          />
          <Textarea
            aria-label="通知正文"
            value={content}
            onChange={(event) => setContent(event.target.value)}
            placeholder="纯文本正文"
            rows={5}
            maxLength={5000}
          />
          <label className="flex items-center gap-2 text-sm text-text-secondary">
            <input type="checkbox" checked={all} onChange={(event) => setAll(event.target.checked)} />
            发送给发布时全部启用用户
          </label>
          {!all && (
            <div className="grid max-h-44 gap-2 overflow-auto rounded-lg border border-border p-3 sm:grid-cols-2">
              {users.map((user) => (
                <label key={user.id} className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={selected.includes(user.id)}
                    onChange={(event) => setSelected((current) => event.target.checked
                      ? [...current, user.id]
                      : current.filter((id) => id !== user.id))}
                  />
                  <span>{user.nickname}（{user.username}）</span>
                </label>
              ))}
            </div>
          )}
          <div>
            <Button variant="primary" disabled={publishing} onClick={() => void publish()}>
              <Send className="h-4 w-4" aria-hidden />
              {publishing ? "发布中…" : "立即发布"}
            </Button>
          </div>
        </div>
      </div>
      <DataTableCard>
        <TableToolbar title="通知历史" description={historyError ? undefined : `共 ${total} 条`} />
        <DataTable
          columns={columns}
          dataSource={records}
          rowKey="id"
          loading={loading}
          error={historyError && <EmptyState title="通知历史加载失败" description={historyError} actionText="重试" onAction={() => void load()} />}
          empty={<EmptyState title="暂无通知" description="已发布的通知会显示在这里。" />}
          minWidth={612}
          compact
          ariaLabel="通知历史"
        />
        <Pagination
          page={page}
          pageSize={10}
          total={total}
          disabled={loading || Boolean(historyError)}
          onPageChange={setPage}
        />
      </DataTableCard>
    </div>
  );
}
