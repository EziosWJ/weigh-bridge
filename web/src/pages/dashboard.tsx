import { Activity, AlertTriangle, CheckCircle2, Clock3 } from "lucide-react";
import { ContentCard } from "@/components/common/content-card";
import { PageHeader } from "@/components/common/page-header";
import { StatusTag } from "@/components/common/status-tag";

const kpis = [
  { label: "今日访问", value: "24,560", change: "+12.4%", tone: "success" },
  { label: "新增用户", value: "1,248", change: "+8.2%", tone: "success" },
  { label: "待处理工单", value: "36", change: "-4", tone: "warning" },
  { label: "系统可用率", value: "99.96%", change: "稳定", tone: "info" },
] as const;

const activities = [
  "管理员更新了用户角色配置",
  "系统完成每日数据同步任务",
  "用户导出了一份运营报表",
  "新增 12 条待审核记录",
];

const todoItems = [
  { label: "待审核用户", count: 18, tone: "warning" as const },
  { label: "异常登录告警", count: 3, tone: "error" as const },
  { label: "配置待发布", count: 5, tone: "info" as const },
];

export function DashboardPage() {
  return (
    <>
      <PageHeader
        title="工作台"
        description="系统运行概览及待处理事项"
      />

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        {kpis.map((item) => (
          <ContentCard key={item.label} bodyClassName="p-4">
            <div className="flex items-start justify-between">
              <div>
                <p className="text-sm text-text-tertiary">{item.label}</p>
                <p className="mt-2 text-2xl font-semibold leading-8">
                  {item.value}
                </p>
              </div>
              <StatusTag tone={item.tone}>{item.change}</StatusTag>
            </div>
          </ContentCard>
        ))}
      </div>

      <div className="mt-6 grid gap-6 xl:grid-cols-[minmax(0,2fr)_minmax(320px,1fr)]">
        <ContentCard
          title="业务趋势"
          description="第一版使用占位图形，后续可接入图表库。"
        >
          <div className="h-72 rounded-lg border border-dashed border-border bg-slate-50 p-5">
            <div className="flex h-full items-end gap-3">
              {[48, 64, 52, 78, 70, 88, 74, 92, 84, 98, 90, 104].map(
                (height, index) => (
                  <div
                    key={index}
                    className="flex flex-1 items-end rounded-t-md bg-blue-100"
                    style={{ height }}
                    aria-hidden
                  >
                    <div className="h-2/3 w-full rounded-t-md bg-primary" />
                  </div>
                ),
              )}
            </div>
          </div>
        </ContentCard>

        <ContentCard title="系统状态 / 待办">
          <div className="space-y-4">
            <div className="grid gap-3">
              <div className="flex items-center gap-3 rounded-lg border border-border px-3 py-3">
                <CheckCircle2 className="h-5 w-5 text-success" aria-hidden />
                <div>
                  <div className="font-medium">核心服务运行正常</div>
                  <div className="text-xs text-text-tertiary">
                    最近检查 2 分钟前
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-3 rounded-lg border border-border px-3 py-3">
                <Activity className="h-5 w-5 text-info" aria-hidden />
                <div>
                  <div className="font-medium">接口平均响应 126ms</div>
                  <div className="text-xs text-text-tertiary">
                    过去 1 小时统计
                  </div>
                </div>
              </div>
            </div>

            <div className="space-y-2">
              {todoItems.map((item) => (
                <div
                  key={item.label}
                  className="flex items-center justify-between rounded-lg bg-slate-50 px-3 py-2"
                >
                  <span className="text-sm">{item.label}</span>
                  <StatusTag tone={item.tone}>{String(item.count)}</StatusTag>
                </div>
              ))}
            </div>
          </div>
        </ContentCard>
      </div>

      <div className="mt-6 grid gap-6 xl:grid-cols-2">
        <ContentCard title="最近操作">
          <ul className="space-y-3">
            {activities.map((item, index) => (
              <li key={item} className="flex items-start gap-3">
                <Clock3 className="mt-0.5 h-4 w-4 text-text-tertiary" />
                <div>
                  <div className="text-sm text-text-primary">{item}</div>
                  <div className="text-xs text-text-tertiary">
                    {index + 1}0 分钟前
                  </div>
                </div>
              </li>
            ))}
          </ul>
        </ContentCard>

        <ContentCard title="风险提示">
          <div className="flex items-start gap-3 rounded-lg border border-amber-100 bg-amber-50 px-4 py-3">
            <AlertTriangle className="mt-0.5 h-5 w-5 text-warning" />
            <div>
              <div className="font-medium text-text-primary">
                有 3 条异常登录告警待确认
              </div>
              <p className="mt-1 text-sm text-text-secondary">
                建议检查近期登录日志和管理员账号安全策略。
              </p>
            </div>
          </div>
        </ContentCard>
      </div>
    </>
  );
}
