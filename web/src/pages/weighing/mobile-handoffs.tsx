import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { PageHeader } from '@/components/common/page-header';
import { ContentCard } from '@/components/common/content-card';
import { StatusTag } from '@/components/common/status-tag';
import { mobileHandoffApi, type WireMobileHandoff } from '@/api/weighing-workflow';
import { beijingTime } from '@/lib/weighing/query';

const actionLabels = { first: '首磅', second: '回磅' } as const;

function statusTone(status: WireMobileHandoff['status']) {
  if (status === 'completed') return 'success' as const;
  if (status === 'failed') return 'error' as const;
  return 'warning' as const;
}

function statusLabel(status: WireMobileHandoff['status']) {
  if (status === 'completed') return '已完成';
  if (status === 'failed') return '失败';
  return '处理中';
}

function weight(value?: string) {
  return value ? `${value} 吨` : '—';
}

function sourceLabel(source?: string) {
  if (source === 'serial_agent') return '串口扫码枪';
  if (source === 'operator') return '手工扫码';
  return source || '—';
}

function resultText(item: WireMobileHandoff) {
  if (item.result?.ticketNo) return item.result.ticketNo;
  if (item.result?.taskNo) return `任务 ${item.result.taskNo}`;
  if (item.error) return item.error;
  return '—';
}

export function MobileHandoffsPage() {
  const [items, setItems] = useState<WireMobileHandoff[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  async function reload() {
    try {
      setItems(await mobileHandoffApi.list());
      setError('');
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : '扫码记录加载失败');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void reload();
    const timer = window.setInterval(() => void reload(), 3000);
    return () => window.clearInterval(timer);
  }, []);

  return (
    <div className="space-y-space-5">
      <PageHeader
        title="手机扫码"
        description="串口扫码 Agent 在厂端后台自动处理手机首磅/回磅二维码。本页为只读扫码记录，用于查看结果和进入任务、磅单详情。"
      />

      <ContentCard title="扫码方式" description="无需打开本页面，也无需聚焦输入框。">
        <p className="text-sm text-text-secondary">
          手机显示 <span className="font-mono">fc-handoff:</span> 二维码后，由串口扫码枪直接扫描。厂端后台 Agent 读取串口并调用内部扫码接口，自动完成首磅建任务或回磅出单。
        </p>
      </ContentCard>

      <ContentCard title="最近扫码记录" description="每 3 秒刷新一次，显示串口 Agent 最近处理的扫码结果。">
        {error && <p role="alert" className="mb-space-3 text-sm text-danger">{error}</p>}
        {loading ? <p className="text-sm text-text-tertiary">正在加载…</p> : items.length === 0 ? <p className="text-sm text-text-tertiary">暂无扫码记录。</p> : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[980px] border-collapse text-sm">
              <thead>
                <tr className="border-b border-border text-left text-text-tertiary">
                  <th className="px-space-2 py-space-2 font-medium">时间</th>
                  <th className="px-space-2 py-space-2 font-medium">动作</th>
                  <th className="px-space-2 py-space-2 font-medium">车牌</th>
                  <th className="px-space-2 py-space-2 font-medium">手机账号</th>
                  <th className="px-space-2 py-space-2 font-medium">来源</th>
                  <th className="px-space-2 py-space-2 font-medium">状态</th>
                  <th className="px-space-2 py-space-2 font-medium">任务 / 磅单</th>
                  <th className="px-space-2 py-space-2 font-medium">首磅 / 回磅 / 净重</th>
                  <th className="px-space-2 py-space-2 font-medium">抓拍</th>
                </tr>
              </thead>
              <tbody>
                {items.map(item => (
                  <tr key={item.id} className="border-b border-border/60 align-top">
                    <td className="px-space-2 py-space-3 whitespace-nowrap">{beijingTime(item.completedAt || item.claimedAt)}</td>
                    <td className="px-space-2 py-space-3">{actionLabels[item.action] || item.action}</td>
                    <td className="px-space-2 py-space-3 font-medium">{item.plate}</td>
                    <td className="px-space-2 py-space-3">{item.mobileAccount || '—'}</td>
                    <td className="px-space-2 py-space-3">{sourceLabel(item.source)}{item.device ? <span className="block text-xs text-text-tertiary">{item.device}</span> : null}</td>
                    <td className="px-space-2 py-space-3"><StatusTag tone={statusTone(item.status)}>{statusLabel(item.status)}</StatusTag></td>
                    <td className="px-space-2 py-space-3">{item.ticketId ? <Link className="text-primary hover:underline" to={`/weighing/tickets/${item.ticketId}`}>{item.result?.ticketNo || '查看磅单'}</Link> : item.taskId ? <Link className="text-primary hover:underline" to={`/weighing/tasks/${item.taskId}`}>{item.result?.taskNo || '查看任务'}</Link> : resultText(item)}</td>
                    <td className="px-space-2 py-space-3 whitespace-nowrap">{weight(item.result?.firstWeight)} / {weight(item.result?.secondWeight)} / {weight(item.result?.net)}</td>
                    <td className="px-space-2 py-space-3">{item.result?.captureStatus || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </ContentCard>
    </div>
  );
}
