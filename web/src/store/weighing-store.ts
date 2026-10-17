import { create } from 'zustand';
import { applyCommand } from '@/lib/weighing/engine';
import { createInitialState } from '@/lib/weighing/seed';
import { migrateWeighingState } from '@/lib/weighing/migration';
import type { WeighingCommand, WeighingState } from '@/types/weighing';
import { useAuthStore } from './auth-store';

export const WEIGHING_STORAGE_KEY = 'weighbridge-phase2a-v1';

function restore(): { data: WeighingState; storageError: string } {
  let data = createInitialState();
  try {
    const raw = localStorage.getItem(WEIGHING_STORAGE_KEY);
    if (raw) data = migrateWeighingState(JSON.parse(raw));
  } catch {
    return { data, storageError: '本模块浏览器缓存读取失败，已恢复测试示例，请确认浏览器允许本地保存。' };
  }
  try {
    localStorage.setItem(WEIGHING_STORAGE_KEY, JSON.stringify(data));
    return { data, storageError: '' };
  } catch {
    return { data, storageError: '浏览器保存失败，当前进度仅在本页内存中；刷新可能丢失。' };
  }
}

export const useWeighingStore = create<{
  data: WeighingState;
  storageError: string;
  dispatch: (command: WeighingCommand) => void;
}>((set, get) => ({
  ...restore(),
  dispatch: (command) => {
    const user = useAuthStore.getState().user;
    const attributed = command.type === 'confirm' || command.type === 'reopenTicket'
      ? { ...command, actor: user ? `${user.nickname || user.username}（${user.username}）· 演示操作` : undefined }
      : command;
    const data = applyCommand(get().data, attributed);
    let storageError = '';
    try {
      localStorage.setItem(WEIGHING_STORAGE_KEY, JSON.stringify(data));
    } catch {
      storageError = '浏览器保存失败，当前进度仅在本页内存中；刷新可能丢失。';
    }
    set({ data, storageError });
  },
}));
