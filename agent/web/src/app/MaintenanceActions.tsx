import { createContext, useContext, useEffect, useRef, useState, type Key, type ReactNode } from 'react';
import { App, Button, Modal, Space } from 'antd';
import { useQueryClient } from '@tanstack/react-query';
import { fetchJobs, keys, post, SessionExpired } from '../api';
import { useMaintenance } from './AppProviders';
import { JobsTable } from '../components/Shared';
import type { ActionResult, ConfigData, JobsData, Status } from '../types';

interface Choice { disposition: string; jobIds: string[]; previewToken: string }
interface Preview { action: string; data: JobsData; resolve: (choice: Choice | null) => void }
interface Actions {
  busy: boolean;
  message: { text: string; failed: boolean } | null;
  run: (work: () => Promise<void>) => Promise<void>;
  confirm: (title: string, body: ReactNode, label: string, danger?: boolean) => Promise<boolean>;
  act: (action: string, extra?: object) => Promise<void>;
  guarded: (action: string) => Promise<void>;
  notify: (text: string) => void;
}
const Context = createContext<Actions | null>(null);
export function useActions() {
  const value = useContext(Context);
  if (!value) throw Error('Missing actions provider');
  return value;
}

export function MaintenanceActions({ children }: { children: ReactNode }) {
  const { modal } = App.useApp();
  const { authenticated, setAuthenticated } = useMaintenance();
  const client = useQueryClient();
  const lock = useRef(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<Actions['message']>(null);
  const [preview, setPreview] = useState<Preview | null>(null);
  const previewRef = useRef<Preview | null>(null);
  const [selected, setSelected] = useState<Key[]>([]);
  useEffect(() => {
    if (!authenticated) {
      modal.destroyAll();
      previewRef.current?.resolve(null);
      previewRef.current = null;
      setPreview(null);
      setMessage(null);
    }
  }, [authenticated, modal]);
  const notify = (text: string) => setMessage({ text, failed: false });
  const refresh = async () => {
    await Promise.all([
      client.invalidateQueries({ queryKey: keys.status }),
      client.invalidateQueries({ queryKey: ['private'] }),
    ]);
  };
  const run: Actions['run'] = async (work) => {
    if (lock.current) return;
    lock.current = true; setBusy(true);
    try { await work(); }
    catch (error) {
      if (error instanceof SessionExpired) setAuthenticated(false);
      setMessage({ text: error instanceof Error ? error.message : String(error), failed: true });
    } finally { lock.current = false; setBusy(false); }
  };
  const confirm: Actions['confirm'] = async (title, content, okText, danger = false) =>
    Boolean(await modal.confirm({ title, content, okText, cancelText: '取消', okButtonProps: { danger }, width: 700, maskClosable: false }));
  const choose = (action: string, data: JobsData) => new Promise<Choice | null>((resolve) => {
    const value = { action, data, resolve };
    previewRef.current = value; setSelected([]); setPreview(value);
  });
  const close = (choice: Choice | null) => {
    previewRef.current?.resolve(choice); previewRef.current = null; setPreview(null);
  };
  const act: Actions['act'] = async (action, extra = {}) => {
    let payload = extra;
    while (true) {
      const result = await post<ActionResult>('/api/action', { action, ...payload });
      if (!result.needsDisposition) {
        notify(result.output || '维护操作已完成，已核对执行结果。纸面结果仍需现场核实。');
        await refresh(); return;
      }
      const choice = await choose(action, { ...result, jobs: result.jobs || [], unknown: !!result.unknown, queuesOutput: '' });
      if (!choice) return;
      payload = choice;
    }
  };
  const guarded: Actions['guarded'] = async (action) => {
    const data = await fetchJobs();
    const choice = await choose(action, data);
    if (choice) await act(action, choice);
  };
  const state = client.getQueryData<Status>(keys.status);
  const config = client.getQueryData<ConfigData>(keys.config);
  const changingConfig = preview?.action === 'apply' || preview?.action === 'restore';
  const queueNames = preview?.action === 'restart-cups'
    ? (preview.data.queuesOutput.split('\n').filter((line) => line.startsWith('printer ')).map((line) => line.split(/\s+/)[1]))
    : [...new Set([state?.queue, ...(changingConfig ? [config?.config.queue] : [])].filter(Boolean))];
  return <Context.Provider value={{ busy, message, run, confirm, act, guarded, notify }}>
    {children}
    <Modal open={!!preview} title={changingConfig ? '配置应用前检查' : '恢复服务前检查'} width={800}
      onCancel={() => close(null)} destroyOnHidden maskClosable={false}
      footer={<Space wrap>
        <Button onClick={() => close(null)}>放弃操作</Button>
        <Button onClick={() => {
          if (changingConfig) { close(null); notify('作业已保留，配置尚未应用。请在作业完成后重试。'); }
          else close({ previewToken: preview!.data.previewToken, disposition: preview!.data.unknown ? 'unknown-confirmed' : 'keep', jobIds: [] });
        }}>{changingConfig ? '保留作业，暂不应用' : preview?.data.unknown ? '接受状态未知并恢复' : '保留作业并继续'}</Button>
        <Button danger disabled={!!preview?.data.unknown || !selected.length} onClick={() => close({
          previewToken: preview!.data.previewToken, disposition: 'cancel', jobIds: selected.map(String),
        })}>取消所选作业后继续</Button>
      </Space>}>
      <p>{preview?.action === 'restart-cups' ? '重启 CUPS 会影响本机全部打印队列。' : '将操作目标服务及相关打印队列。'}</p>
      <p>涉及队列：{queueNames.join('、') || '未知／不可读取'}</p>
      <p>保留的旧作业可能继续出纸；取消不代表此前没有出纸。</p>
      <p>{preview?.data.unknown ? '当前作业未知／不可读取，恢复后的出纸情况需现场检查。' : `检查发现 ${preview?.data.jobs?.length || 0} 个未结束作业，请选择需要取消的作业。`}</p>
      {!!preview?.data.jobs?.length && <JobsTable jobs={preview.data.jobs} selected={selected} onSelect={setSelected} />}
    </Modal>
  </Context.Provider>;
}
