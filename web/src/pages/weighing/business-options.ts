import { useCallback, useMemo } from 'react';
import { weighingApi } from '@/api/weighing';
import { useAuthStore } from '@/store/auth-store';
import { useWeighingStore } from '@/store/weighing-store';
import type { Party, PartyKind, TaskInput } from '@/types/weighing';
import { partyRoleLabels, partyNatureLabels } from '@/types/weighing';

export function isAvailableParty(party: Party) {
  return party.enabled && party.temporaryState !== 'merged' && party.temporaryState !== 'disabled';
}

export function tripSupplierProblem(party: Party | undefined) {
  if (!party || party.temporaryState) return '指定正式供货方尚未同步，请先同步基础资料。';
  if (!isAvailableParty(party)) return '指定正式供货方已停用，请核对流转单。';
  if (!party.roles.includes('supplier')) return '指定正式供货方缺少供货商身份，请核对总部资料。';
  return '';
}

function partyOption(party: Party, roles: PartyKind[], matched = false) {
  return {
    id: party.id,
    name: party.name,
    code: party.code,
    details: [party.shortName, party.phone, party.code].filter(Boolean),
    tags: [
      ...(party.temporaryState ? ['临时'] : []),
      partyNatureLabels[party.nature],
      ...roles.map(role => partyRoleLabels[role]),
      ...(matched ? ['关联物料匹配'] : []),
    ],
    disabled: !isAvailableParty(party),
    remark: party.remark,
    matched,
  };
}

/** 物料与往来主体的现场选择数据；工作台和资料表单共用同一套有效性过滤。 */
export function useBusinessOptions(value: TaskInput, recommendSuppliers = false) {
  const { materials, parties } = useWeighingStore(s => s.data);
  const materialOptions = useMemo(() => materials.filter(m => m.enabled || m.id === value.materialId).map(m => ({ id: m.id, name: m.name, code: m.code, details: [m.spec || '未填写规格', m.code], tags: ['吨'], disabled: !m.enabled })), [materials, value.materialId]);
  const materialName = recommendSuppliers && value.direction === 'inbound' ? materials.find(m => m.id === value.materialId)?.name.trim() : '';
  const makePartyOptions = useCallback((items: Party[]) => items
    .filter(p => (isAvailableParty(p) || p.id === value.partyId)
      && p.roles.includes(value.direction === 'inbound' ? 'supplier' : 'customer'))
    .map(p => partyOption(p, p.roles, Boolean(materialName && p.remark?.includes(materialName))))
    .sort((a, b) => Number(b.matched) - Number(a.matched)), [materialName, value.partyId, value.direction]);
  const makeCarrierOptions = useCallback((items: Party[]) => items
    .filter(p => (isAvailableParty(p) || p.id === value.carrierId) && p.roles.includes('carrier'))
    .map(p => partyOption(p, ['carrier'])), [value.carrierId]);
  const partyOptions = useMemo(() => makePartyOptions(parties), [makePartyOptions, parties]);
  const carrierOptions = useMemo(() => makeCarrierOptions(parties), [makeCarrierOptions, parties]);
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
  const loadPartyOptions = useCallback(async () => makePartyOptions(await loadParties()), [loadParties, makePartyOptions]);
  const loadCarrierOptions = useCallback(async () => makeCarrierOptions(await loadParties()), [loadParties, makeCarrierOptions]);
  return { materials, parties, materialOptions, partyOptions, carrierOptions, loadMaterials, loadParties, loadPartyOptions, loadCarrierOptions };
}

export function supplementarySummary(value: TaskInput, parties: Party[]) {
  const carrier = parties.find(p => p.id === value.carrierId);
  return [carrier && `承运：${carrier.name}`, value.contactName && `联系人：${value.contactName}`, value.driver && `司机：${value.driver}`, value.driverPhone && `司机电话：${value.driverPhone}`, value.remark && `备注：${value.remark}`].filter(Boolean).join(' · ') || '按需填写承运方、联系人、司机或备注';
}
