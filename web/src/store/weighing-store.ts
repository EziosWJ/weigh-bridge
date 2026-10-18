import { create } from 'zustand';
import { siteConfigKeys } from '@/lib/weighing/plate';
import { applyCommand } from '@/lib/weighing/engine';
import { createInitialState } from '@/lib/weighing/seed';
import { migrateWeighingState } from '@/lib/weighing/migration';
import type { WeighingCommand, WeighingState } from '@/types/weighing';
import { useAuthStore } from './auth-store';

export const WEIGHING_STORAGE_KEY = 'weighbridge-phase2a-v1';
const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

function restore(): { data: WeighingState; storageError: string } {
  try {
    const raw = localStorage.getItem(WEIGHING_STORAGE_KEY);
    // 加载不回写，避免新标签抹掉另一标签已采集的依据。
    return { data: raw ? migrateWeighingState(JSON.parse(raw)) : createInitialState(), storageError: '' };
  } catch {
    return { data: createInitialState(), storageError: '本模块浏览器缓存读取失败，请确认浏览器允许本地保存。' };
  }
}

/** 资料同步不触碰本页现场草稿；当前任务或现场条件变化才使采集失效。 */
function mergeShared(local: WeighingState, shared: WeighingState): WeighingState {
  if (shared.resetAt !== local.resetAt) return { ...shared, capture: undefined, sampleReady: false };
  const plate = local.capture?.plate || local.draft.plate;
  const related = (data: WeighingState) => data.tasks.filter(task => task.plate === plate);
  const invalid = siteConfigKeys.some(key => !same(local.config[key], shared.config[key])) || !same(related(local), related(shared)) || shared.owner === 'other';
  return { ...shared, draft: local.draft, draftRequestId: local.draftRequestId,
    draftPlateEvidence: local.draftPlateEvidence, capture: invalid ? undefined : local.capture, sampleReady: !invalid && local.sampleReady };
}

export const useWeighingStore = create<{
  data: WeighingState;
  storageError: string;
  syncNotice: string;
  dispatch: (command: WeighingCommand, expected?: WeighingState) => Promise<void>;
}>((set, get) => ({
  ...restore(), syncNotice: '',
  dispatch: async (command, expected) => {
    const commit = () => {
      const local = get().data;
      let shared = local;
      try {
        const raw = localStorage.getItem(WEIGHING_STORAGE_KEY);
        if (raw) {
          const saved = JSON.parse(raw) as WeighingState;
          shared = saved.version === 3 ? saved : migrateWeighingState(saved);
        }
      } catch { /* 保存失败仍保留本页可用状态，并在下方明确提示。 */ }
      const base = mergeShared(local, shared);
      if (expected && (!same(expected.config, base.config) || !same(expected.tasks, base.tasks) || !same(expected.draft, base.draft) || !same(expected.capture, base.capture))) {
        set({ data: base });
        throw new Error('数据已变化，请重新采集并核对。');
      }
      const user = useAuthStore.getState().user;
      const attributed = { ...command, actor: user ? `${user.nickname || user.username}（${user.username}）· 演示操作` : undefined };
      const data = applyCommand(base, attributed);
      // 后台资料维护只写它改变的工作区字段，不用自身旧草稿覆盖工作台。
      const workspaceCommand = ['draft', 'verifyPlate', 'clearDraft', 'capture', 'firstWeigh', 'confirm', 'resolveSubmission', 'reset'].includes(command.type);
      const invalidatesShared = siteConfigKeys.some(key => !same(base.config[key], data.config[key])) || (data.owner === 'other') || !same(base.tasks, data.tasks);
      const persisted = workspaceCommand ? data : { ...data, draft: shared.draft, draftRequestId: shared.draftRequestId, draftPlateEvidence: shared.draftPlateEvidence,
        capture: invalidatesShared ? undefined : shared.capture, sampleReady: !invalidatesShared && shared.sampleReady };
      let storageError = '';
      try { localStorage.setItem(WEIGHING_STORAGE_KEY, JSON.stringify(persisted)); }
      catch { storageError = '浏览器保存失败，当前进度仅在本页内存中；刷新可能丢失。'; }
      set({ data, storageError, syncNotice: '' });
    };
    // 同源标签串行保存，再读取最新状态应用业务命令。
    if (navigator.locks) await navigator.locks.request(WEIGHING_STORAGE_KEY, commit);
    else commit();
  },
}));

window.addEventListener('storage', event => {
  if (event.key !== WEIGHING_STORAGE_KEY || !event.newValue) return;
  try {
    const shared = JSON.parse(event.newValue) as WeighingState;
    const local = useWeighingStore.getState().data;
    const data = mergeShared(local, shared);
    const invalidated = local.sampleReady && !data.sampleReady;
    useWeighingStore.setState({ data, syncNotice: invalidated ? '后台已修改当前任务或模拟现场条件，请重新采集并核对。' : useWeighingStore.getState().syncNotice });
  } catch { useWeighingStore.setState({ storageError: '其他标签数据同步失败，请检查浏览器存储。' }); }
});
