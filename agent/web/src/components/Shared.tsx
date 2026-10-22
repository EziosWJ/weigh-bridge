import { Alert, Button, Card, Table, Tag } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { fetchStatus, keys } from '../api';
import type { Job } from '../types';
import type { Key } from 'react';

export function useStatus() {
  return useQuery({ queryKey: keys.status, queryFn: fetchStatus, refetchInterval: 15000 });
}
export function StatusTag({ value }: { value: string }) {
  const color = /运行中|已启用|已连接|稳定/.test(value) ? 'success' : /故障|过期|未运行|暂停|停止|断开/.test(value) ? 'error' : 'default';
  return <Tag color={color}>{value}</Tag>;
}
export function QueryError({ error, retry }: { error: Error | null; retry: () => unknown }) {
  return error ? <Alert type="error" showIcon title={error.message} action={<Button size="small" onClick={() => retry()}>重试</Button>} /> : null;
}
export function PrivateGate() {
  return <Card><p>请先使用本地维护密码登录，再查看配置、完整日志或执行维护操作。</p></Card>;
}
export function JobsTable({ jobs, selected, onSelect }: { jobs: Job[]; selected?: Key[]; onSelect?: (keys: Key[]) => void }) {
  return <Table<Job> rowKey="id" size="small" dataSource={jobs} pagination={false} scroll={{ x: 560 }}
    rowSelection={onSelect ? { selectedRowKeys: selected, onChange: onSelect, columnTitle: '选择' } : undefined}
    columns={[
      { title: '作业编号', dataIndex: 'id' }, { title: '所属队列', dataIndex: 'queue' },
      { title: '提交信息', render: (_, job) => job.title || '纸面结果未知' },
    ]} locale={{ emptyText: '当前无未结束作业。' }} />;
}
export function stamp(value?: string) {
  return value && !value.startsWith('0001-') ? new Date(value).toLocaleString() : '尚无';
}
