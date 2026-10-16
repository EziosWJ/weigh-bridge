import { AlertCircle, CheckCircle2, FileUp, Image as ImageIcon, Upload } from "lucide-react";
import { useRef, useState, type ChangeEvent, type FormEvent } from "react";
import { uploadFiles } from "@/api/file";
import { FileUpload } from "@/components/common/file-upload";
import { ContentCard } from "@/components/common/content-card";
import { PageHeader } from "@/components/common/page-header";
import { StatusTag } from "@/components/common/status-tag";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { useAuthenticatedFileUrl } from "@/hooks/use-authenticated-file-url";
import { getErrorMessage } from "@/lib/api-error";
import type { FileRecord, FileUploadBatchResult } from "@/types";
import { formatFileSize } from "../system/files/utils";

function BatchUploadDemo() {
  const inputRef = useRef<HTMLInputElement>(null);
  const [files, setFiles] = useState<File[]>([]);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<FileUploadBatchResult | null>(null);

  const handleFileChange = (event: ChangeEvent<HTMLInputElement>) => {
    setFiles(Array.from(event.target.files ?? []));
    setError("");
    setResult(null);
  };

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (files.length === 0) {
      setError("请选择至少一个文件");
      return;
    }

    setUploading(true);
    setError("");

    try {
      const nextResult = await uploadFiles(files, {
        businessModule: "upload-demo-batch",
      });
      const mergedResult: FileUploadBatchResult = {
        succeeded: [...(result?.succeeded ?? []), ...nextResult.succeeded],
        failed: nextResult.failed,
      };
      setResult(mergedResult);

      if (nextResult.failed.length > 0) {
        const failedIndices = new Set(nextResult.failed.map((item) => item.index));
        setFiles((current) => current.filter((_, index) => failedIndices.has(index)));
        toast.warning({
          title: "部分文件上传失败",
          description: `${nextResult.succeeded.length} 个成功，${nextResult.failed.length} 个失败。可继续重试失败项。`,
        });
      } else {
        setFiles([]);
        toast.success(`已成功上传 ${nextResult.succeeded.length} 个文件`);
      }
    } catch (uploadError) {
      setError(getErrorMessage(uploadError, "批量上传失败"));
    } finally {
      setUploading(false);
    }
  };

  return (
    <ContentCard
      title="多文件上传"
      description="一次提交多个文件，部分失败时保留失败项并可重试。"
      className="h-full"
    >
      <form className="flex h-full flex-col" onSubmit={(event) => void handleSubmit(event)}>
        <input
          ref={inputRef}
          type="file"
          multiple
          className="sr-only"
          onChange={handleFileChange}
        />
        <div className="flex items-center gap-3">
          <Button
            type="button"
            variant="secondary"
            disabled={uploading}
            onClick={() => inputRef.current?.click()}
          >
            <Upload className="h-4 w-4" aria-hidden />
            选择多个文件
          </Button>
          <span className="min-w-0 flex-1 truncate text-xs text-text-tertiary">
            {files.length > 0 ? `待上传 ${files.length} 个文件` : "尚未选择文件"}
          </span>
        </div>

        {files.length > 0 && (
          <ul className="mt-3 max-h-32 space-y-1 overflow-y-auto rounded-lg border border-border bg-slate-50 px-3 py-2 text-xs text-text-secondary">
            {files.map((file, index) => (
              <li key={`${file.name}-${index}`} className="flex items-center justify-between gap-3">
                <span className="min-w-0 truncate">{index + 1}. {file.name}</span>
                <span className="shrink-0 text-text-tertiary">{formatFileSize(file.size)}</span>
              </li>
            ))}
          </ul>
        )}

        <div className="mt-4 flex-1">
          {result && (
            <div className="space-y-3 rounded-lg border border-border bg-slate-50 p-3 text-xs" aria-live="polite">
              <div className="flex items-center justify-between gap-3">
                <span className="font-medium text-text-primary">本次结果</span>
                <StatusTag tone={result.failed.length > 0 ? "warning" : "success"}>
                  {`成功 ${result.succeeded.length} · 失败 ${result.failed.length}`}
                </StatusTag>
              </div>
              {result.succeeded.length > 0 && (
                <ul className="space-y-1 text-success">
                  {result.succeeded.map((file) => (
                    <li key={file.id} className="flex items-center gap-1.5 truncate">
                      <CheckCircle2 className="h-3.5 w-3.5 shrink-0" aria-hidden />
                      <span className="truncate">{file.originalName}</span>
                    </li>
                  ))}
                </ul>
              )}
              {result.failed.length > 0 && (
                <ul className="space-y-1 text-error" role="alert">
                  {result.failed.map((item) => (
                    <li key={`${item.index}-${item.fileName}`} className="flex items-start gap-1.5">
                      <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden />
                      <span className="min-w-0 break-words">第 {item.index + 1} 项 · {item.fileName}：{item.message}</span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
          {error && <p className="mt-3 text-xs text-error" role="alert">{error}</p>}
        </div>

        <div className="mt-5 border-t border-border pt-4">
          <Button type="submit" variant="primary" disabled={uploading || files.length === 0}>
            <FileUp className="h-4 w-4" aria-hidden />
            {uploading ? "上传中..." : result?.failed.length ? "重试失败项" : "上传所选文件"}
          </Button>
        </div>
      </form>
    </ContentCard>
  );
}

function ImageUploadDemo() {
  const [record, setRecord] = useState<FileRecord | null>(null);
  const {
    url,
    loading,
    error,
  } = useAuthenticatedFileUrl(record?.accessUrl);

  return (
    <ContentCard
      title="图片上传"
      description="上传后通过受保护的预览流展示图片内容。"
      className="h-full"
    >
      <FileUpload
        accept="image/png,image/jpeg,image/gif"
        buttonText="选择图片"
        helperText="支持 PNG、JPEG、GIF；服务端会校验真实图片内容。"
        businessModule="upload-demo-image"
        onUploaded={setRecord}
        onCleared={() => setRecord(null)}
      />

      <div className="mt-4 flex min-h-36 items-center justify-center overflow-hidden rounded-lg border border-dashed border-border bg-slate-50 p-3">
        {loading && <span className="text-xs text-text-tertiary">加载图片预览...</span>}
        {error && <p className="text-xs text-error" role="alert">{error}</p>}
        {!loading && !error && url && (
          <img src={url} alt={record?.originalName ?? "已上传图片"} className="max-h-44 max-w-full rounded object-contain" />
        )}
        {!loading && !error && !url && (
          <div className="flex flex-col items-center gap-2 text-xs text-text-tertiary">
            <ImageIcon className="h-7 w-7" aria-hidden />
            上传成功后在此预览
          </div>
        )}
      </div>
    </ContentCard>
  );
}

export function FileUploadDemoPage() {
  return (
    <>
      <PageHeader
        title="文件上传 Demo"
        description="三个示例均连接登录态真实接口，用于核对上传状态、结果、错误和预览链路。"
      />

      <div className="space-y-6">
        <div className="flex flex-col gap-3 rounded-admin border border-blue-100 bg-blue-50 px-4 py-3 text-sm text-blue-900 md:flex-row md:items-center md:justify-between">
          <div>
            <p className="font-medium">受保护的文件服务</p>
            <p className="mt-1 text-xs text-blue-800/80">请求自动携带当前登录态，图片预览使用带 Bearer 的文件流。</p>
          </div>
          <StatusTag tone="info">Bearer 登录态</StatusTag>
        </div>

        <div className="grid gap-6 xl:grid-cols-3">
          <ContentCard
            title="单文件上传"
            description="提交一个文件并展示服务端返回的文件记录。"
            className="h-full"
          >
            <FileUpload
              buttonText="选择单文件"
              helperText="POST /api/system/file/upload · 单文件最大 50 MB。"
              businessModule="upload-demo-single"
            />
          </ContentCard>

          <BatchUploadDemo />
          <ImageUploadDemo />
        </div>
      </div>
    </>
  );
}
