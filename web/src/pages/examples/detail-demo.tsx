import { ArrowLeft, CheckCircle2, FileText, Pencil } from "lucide-react";
import { ContentCard } from "@/components/common/content-card";
import { PageHeader } from "@/components/common/page-header";
import { StatusTag } from "@/components/common/status-tag";
import { Button } from "@/components/ui/button";
import { formatDateTime } from "@/lib/datetime";

const detail = {
  code: "ORD-20260502001",
  title: "企业客户开通申请",
  status: "processing",
  owner: "客户运营部",
  applicant: "上海示例科技有限公司",
  amount: "¥128,000.00",
  createdAt: "2026-05-02 10:12:00",
  updatedAt: "2026-05-02 16:40:00",
};

export function DetailDemoPage() {
  return (
    <>
      <PageHeader
        title="详情页 Demo"
        description="展示后台详情页的摘要、分组信息、状态和操作布局。"
        actions={
          <>
            <Button variant="secondary">
              <ArrowLeft className="h-4 w-4" aria-hidden />
              返回
            </Button>
            <Button variant="primary">
              <Pencil className="h-4 w-4" aria-hidden />
              编辑
            </Button>
          </>
        }
      />

      <div className="space-y-6">
        <ContentCard>
          <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
            <div>
              <div className="flex items-center gap-2">
                <FileText className="h-5 w-5 text-primary" aria-hidden />
                <h2 className="text-lg font-semibold text-text-primary">
                  {detail.title}
                </h2>
                <StatusTag tone="warning">处理中</StatusTag>
              </div>
              <p className="mt-2 text-sm text-text-tertiary">
                单据编号：{detail.code}
              </p>
            </div>
            <div className="rounded-lg border border-border bg-slate-50 px-4 py-3 text-right">
              <div className="text-[13px] text-text-tertiary">申请金额</div>
              <div className="mt-1 text-xl font-semibold tabular-nums text-text-primary">
                {detail.amount}
              </div>
            </div>
          </div>
        </ContentCard>

        <ContentCard title="基础信息" description="适用于订单、用户、设备等详情页的只读信息展示。">
          <div className="grid gap-4 md:grid-cols-3">
            <DetailField label="申请主体" value={detail.applicant} />
            <DetailField label="归属部门" value={detail.owner} />
            <DetailField label="当前状态" value={<StatusTag tone="warning">处理中</StatusTag>} />
            <DetailField label="创建时间" value={formatDateTime(detail.createdAt)} />
            <DetailField label="更新时间" value={formatDateTime(detail.updatedAt)} />
            <DetailField
              label="校验结果"
              value={
                <span className="inline-flex items-center gap-1 text-success">
                  <CheckCircle2 className="h-4 w-4" aria-hidden />
                  已通过
                </span>
              }
            />
          </div>
        </ContentCard>

        <ContentCard title="分组信息" description="复杂详情页建议按业务含义拆分多个区块。">
          <div className="grid gap-4 md:grid-cols-2">
            <DetailField label="合同周期" value="2026-05-01 至 2027-04-30" />
            <DetailField label="服务等级" value="企业版 SLA" />
            <DetailField label="联系人" value="王敏 / 13800000000" />
            <DetailField label="备注" value="开通后需要同步初始化客户权限。" />
          </div>
        </ContentCard>
      </div>
    </>
  );
}

function DetailField({ label, value }: { label: string; value?: React.ReactNode }) {
  return (
    <div>
      <div className="text-[13px] text-text-tertiary">{label}</div>
      <div className="mt-1 break-words text-sm text-text-primary">{value ?? "-"}</div>
    </div>
  );
}
