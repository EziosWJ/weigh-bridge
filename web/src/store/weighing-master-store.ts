import { useEffect } from 'react';
import { create } from 'zustand';
import { weighingApi, type WeighingCapabilities } from '@/api/weighing';
import { useAuthStore } from '@/store/auth-store';
import type { Material, MaterialCategory, Party, WeighingCommand } from '@/types/weighing';

type MasterCommand = Extract<WeighingCommand, { type: 'saveCategory' | 'saveMaterial' | 'saveParty' | 'deleteCategory' | 'deleteMaterial' | 'deleteParty' }> & { version?: number };
interface MasterStore {
  data: { categories: MaterialCategory[]; materials: Material[]; parties: Party[] };
  capabilities: WeighingCapabilities | null;
  loadedToken: string | null;
  loading: boolean;
  saving: boolean;
  error: string;
  load: () => Promise<void>;
  dispatch: (command: MasterCommand) => Promise<void>;
}
let pending: { token: string; promise: Promise<void> } | null = null;
export const useWeighingMasterStore = create<MasterStore>((set, get) => ({
  data: { categories: [], materials: [], parties: [] }, capabilities: null,
  loadedToken: null, loading: false, saving: false, error: '',
  load: () => {
    const token = useAuthStore.getState().token;
    if (!token) return Promise.resolve();
    if (pending?.token === token) return pending.promise;
    set({ loading: true, error: '' });
    const promise = (async () => {
      try {
        const capabilities = await weighingApi.capabilities();
        const canRead = capabilities.permissionCodes.includes('weighing:master:read');
        const [categories, materials, parties] = canRead
          ? await Promise.all([weighingApi.categories(), weighingApi.materials(), weighingApi.parties()])
          : [[], [], []] as [MaterialCategory[], Material[], Party[]];
        if (useAuthStore.getState().token === token) set({ data: { categories, materials, parties }, capabilities, loadedToken: token, loading: false });
      } catch (error) {
        if (useAuthStore.getState().token === token) set({ error: error instanceof Error ? error.message : '资料加载失败', loading: false });
      } finally { if (pending?.token === token) pending = null; }
    })();
    pending = { token, promise };
    return promise;
  },
  dispatch: async command => {
    if (get().saving) throw new Error('正在保存，请稍候');
    set({ saving: true });
    try {
      switch (command.type) {
        case 'saveCategory': await weighingApi.save('material-categories', command.id, command); break;
        case 'saveMaterial': await weighingApi.save('materials', command.id, { ...command, categoryId: command.categoryId ? Number(command.categoryId) : null }); break;
        case 'saveParty': await weighingApi.save('parties', command.id, { ...command, contacts: (command.contacts ?? []).map(c => ({ ...c, id: /^\d+$/.test(c.id) ? Number(c.id) : 0 })) }); break;
        case 'deleteCategory': await weighingApi.remove('material-categories', command.id, command.version); break;
        case 'deleteMaterial': await weighingApi.remove('materials', command.id, command.version); break;
        case 'deleteParty': await weighingApi.remove('parties', command.id, command.version); break;
      }
      await get().load();
      if (get().error) throw new Error(`操作已保存，刷新资料失败：${get().error}。请刷新页面，不要重复新增。`);
    } finally { set({ saving: false }); }
  },
}));

export function useWeighingMasterData() {
  const store = useWeighingMasterStore();
  const token = useAuthStore(s => s.token);
  const load = store.load;
  useEffect(() => { void load(); }, [token, load]);
  return store;
}
