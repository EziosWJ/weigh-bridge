import { useCallback, useMemo } from 'react';
import { weighingApi } from '@/api/weighing';
import { useAuthStore } from '@/store/auth-store';
import { useWeighingStore } from '@/store/weighing-store';
import type { Party, TaskInput } from '@/types/weighing';
import { partyRoleLabels, partyNatureLabels } from '@/types/weighing';

/** 物料与往来主体的现场选择数据；工作台和资料表单共用同一套有效性过滤。 */
export function useBusinessOptions(value: TaskInput) {
  const { materials, parties } = useWeighingStore(s => s.data);
  const materialOptions = useMemo(() => materials.filter(m => m.enabled || m.id === value.materialId).map(m => ({ id: m.id, name: m.name, code: m.code, details: [m.spec || '未填写规格', m.code], tags: ['吨'], disabled: !m.enabled })), [materials, value.materialId]);
  const partyOptions = useMemo(() => parties.filter(p => (p.enabled || p.id === value.partyId) && p.roles.includes(value.direction === 'inbound' ? 'supplier' : 'customer')).map(p => ({ id: p.id, name: p.name, code: p.code, details: [p.shortName, p.phone, p.code].filter(Boolean), tags: [partyNatureLabels[p.nature], ...p.roles.map(r => partyRoleLabels[r])], disabled: !p.enabled })), [parties, value.partyId, value.direction]);
  const carrierOptions = useMemo(() => parties.filter(p => (p.enabled || p.id === value.carrierId) && p.roles.includes('carrier')).map(p => ({ id: p.id, name: p.name, code: p.code, details: [p.shortName, p.phone, p.code].filter(Boolean), tags: [partyNatureLabels[p.nature], '承运商'], disabled: !p.enabled })), [parties, value.carrierId]);
  const loadMaterials = useCallback(async () => {
    const token = useAuthStore.getState().token;
    const materials = await weighingApi.materials();
    if (token !== useAuthStore.getState().token) throw new Error('登录状态已变化，请重新打开选择框');
    useWeighingStore.setState(s => ({ data: { ...s.data, materials } }));
    return materials.filter(m => m.enabled || m.id === value.materialId).map(m => ({ id: m.id, name: m.name, code: m.code, details: [m.spec || '未填写规格', m.code], tags: ['吨'], disabled: !m.enabled }));
  }, [value.materialId]);
  const loadParties = useCallback(async () => {
    const token = useAuthStore.getState().token;
    const parties = await weighingApi.parties();
    if (token !== useAuthStore.getState().token) throw new Error('登录状态已变化，请重新打开选择框');
    useWeighingStore.setState(s => ({ data: { ...s.data, parties } }));
    return parties;
  }, []);
  const loadPartyOptions = useCallback(async () => (await loadParties())
    .filter(p => (p.enabled || p.id === value.partyId) && p.roles.includes(value.direction === 'inbound' ? 'supplier' : 'customer'))
    .map(p => ({ id: p.id, name: p.name, code: p.code, details: [p.shortName, p.phone, p.code].filter(Boolean), tags: [partyNatureLabels[p.nature], ...p.roles.map(r => partyRoleLabels[r])], disabled: !p.enabled })), [loadParties, value.partyId, value.direction]);
  const loadCarrierOptions = useCallback(async () => (await loadParties())
    .filter(p => (p.enabled || p.id === value.carrierId) && p.roles.includes('carrier'))
    .map(p => ({ id: p.id, name: p.name, code: p.code, details: [p.shortName, p.phone, p.code].filter(Boolean), tags: [partyNatureLabels[p.nature], '承运商'], disabled: !p.enabled })), [loadParties, value.carrierId]);
  return { materials, parties, materialOptions, partyOptions, carrierOptions, loadMaterials, loadParties, loadPartyOptions, loadCarrierOptions };
}

export function supplementarySummary(value: TaskInput, parties: Party[]) {
  const carrier = parties.find(p => p.id === value.carrierId);
  return [carrier && `承运：${carrier.name}`, value.contactName && `联系人：${value.contactName}`, value.driver && `司机：${value.driver}`, value.driverPhone && `司机电话：${value.driverPhone}`, value.remark && `备注：${value.remark}`].filter(Boolean).join(' · ') || '按需填写承运方、联系人、司机或备注';
}
