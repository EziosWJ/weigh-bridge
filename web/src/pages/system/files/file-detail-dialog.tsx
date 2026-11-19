import { useEffect, useState } from "react";
import { getFileAccessUrl, getFileDetail } from "@/api/file";
import { ApiStatusTag } from "@/components/common/api-status-tag";
import { DetailDialog } from "@/components/common/detail-dialog";
import { Button } from "@/components/ui/button";
import { DetailItem } from "@/components/common/detail-item";
import type { FileRecord } from "@/types";
import { formatDateTime } from "@/lib/datetime";
import { getErrorMessage } from "@/lib/api-error";
import { formatFileSize } from "./utils";

type FileDetailDialogProps = {
  open: boolean;
  record: FileRecord | null;
  onCancel: () => void;
};

export function FileDetailDialog({
  open,
  record,
  onCancel,
}: FileDetailDialogProps) {
  const [detail, setDetail] = useState<FileRecord | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    if (!open || !record) return;
    let closed = false;
    setDetail(null);
    setLoading(true);
    setError("");

    getFileDetail(record.id)
      .then((data) => { if (!closed) setDetail(data); })
      .catch((err) => { if (!closed) setError(getErrorMessage(err, "文件详情加载失败")); })
      .finally(() => { if (!closed) setLoading(false); });

    return () => { closed = true; };
  }, [open, record, attempt]);

  const data = detail?.id === record?.id ? detail : record;
  const accessUrl = data?.accessUrl ? getFileAccessUrl(data.accessUrl) : "";

  return (
    <DetailDialog
      open={open}
      title="文件详情"
      description={data?.originalName}
      loading={loading}
      trapFocus
      onCancel={onCancel}
    >
      {error && (
        <div className="mb-4 flex flex-wrap items-center gap-2 rounded-control bg-error-background p-3" role="alert">
          <p className="text-sm text-error">{error}。下方显示列表中的信息。</p>
          <Button variant="secondary" size="sm" onClick={() => setAttempt((current) => current + 1)}>重试</Button>
        </div>
      )}
      <div className="grid gap-4 sm:grid-cols-2">
        <DetailItem label="ID" value={data?.id} />
        <DetailItem label="原始文件名" value={data?.originalName} />
        <DetailItem label="存储文件名" value={data?.storageName} />
        <DetailItem label="扩展名" value={data?.extension} />
        <DetailItem label="MIME 类型" value={data?.mimeType} />
        <DetailItem label="文件大小" value={formatFileSize(data?.fileSize)} />
        <DetailItem label="业务模块" value={data?.businessModule} />
        <DetailItem label="备注" value={data?.remark} />
        <DetailItem
          label="状态"
          value={data ? <ApiStatusTag status={data.status} /> : undefined}
        />
        <DetailItem label="创建时间" value={formatDateTime(data?.createTime)} />
        <DetailItem
          label="访问地址"
          className="sm:col-span-2"
          value={
            accessUrl ? (
              <a
                href={accessUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="break-all text-primary underline"
              >
                {accessUrl}
              </a>
            ) : undefined
          }
        />
      </div>
    </DetailDialog>
  );
}
