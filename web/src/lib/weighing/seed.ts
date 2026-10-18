import { getCameraPreset } from './camera';
import { emptyTaskInput } from '../../types/weighing';
import type { Direction, TransportTask, TestTicket, WeighingState } from '../../types/weighing';

/** 本模块全部为浏览器内测试资料，不代表任何正式业务记录。 */
export function createInitialState(now = new Date().toISOString()): WeighingState {
  const at = (minutes: number) => new Date(Date.parse(now) - minutes * 60_000).toISOString();
  const task = (id: string, plate: string, direction: Direction, status: TransportTask['status'], minutes: number): TransportTask => ({
    id, plate, direction, materialId: direction === 'inbound' ? 'material-1' : 'material-2',
    materialName: direction === 'inbound' ? '酿酒高粱' : '酒糟',
    partyId: direction === 'inbound' ? 'party-1' : 'party-2',
    partyName: direction === 'inbound' ? '示例粮食供应商' : '示例饲料收购方', driver: '示例司机', remark: '仅供流程演示',
    materialCode: direction === 'inbound' ? 'M0001' : 'M0002', materialSpec: direction === 'inbound' ? '一级' : '湿酒糟', materialCategoryName: direction === 'inbound' ? '酿酒原料' : '副产物',
    partyCode: direction === 'inbound' ? 'B0001' : 'B0002', partyNature: 'enterprise', partyPhone: '13800000001', partyAddress: '示例地址',
    carrierId: 'party-3', carrierName: '示例运输合作社', carrierCode: 'B0003', contactName: '示例联系人', contactPhone: '13800000001', driverPhone: '13800000009',
    createdAt: at(minutes), status, records: [], history: [{ at: at(minutes), role: 'operator', action: '创建示例测试任务' }], example: true,
  });
  const waitingIn = task('TEST-T-001', '晋A10001', 'inbound', 'second', 50);
  const waitingOut = task('TEST-T-002', '晋A10002', 'outbound', 'second', 40);
  const completed = task('TEST-T-003', '晋A10003', 'inbound', 'completed', 120);
  const terminated = task('TEST-T-004', '晋A10004', 'outbound', 'terminated', 90);
  const voided = task('TEST-T-005', '晋A10005', 'outbound', 'completed', 150);
  const first = task('TEST-T-006', '晋A10006', 'inbound', 'first', 5);
  const crossDay = task('TEST-T-007', '晋A10007', 'inbound', 'second', 26 * 60);
  crossDay.remark = '示例：过夜车，核对昨日首次依据后直接回磅，无需额外说明';
  const snapshot = (task: TransportTask, stage: 'first' | 'second', weight: number, time: string) => {
    const preset = getCameraPreset(stage === 'first' ? 'front' : 'rear');
    return { id: `${task.id}-${stage}-C`, preset: preset.id, imageSrc: preset.src, cameraLabel: preset.title, at: time, weight, plate: task.plate };
  };
  for (const t of [waitingIn, waitingOut, completed, terminated, voided, crossDay]) {
    t.records.push({ id: `${t.id}-W1`, stage: 'first', kind: t.direction === 'inbound' ? 'gross' : 'tare', weight: t.direction === 'inbound' ? 32 : 12, at: t.createdAt, evidence: 'simulated', capture: snapshot(t, 'first', t.direction === 'inbound' ? 32 : 12, t.createdAt) });
    t.history.push({ at: t.createdAt, role: 'operator', action: '确认示例首次测试称重' });
  }
  const ticket = (t: TransportTask, id: string, status: TestTicket['status'], minutes: number): TestTicket => {
    t.records.push({ id: `${t.id}-W2`, stage: 'second', kind: t.direction === 'inbound' ? 'tare' : 'gross', weight: t.direction === 'inbound' ? 12 : 32, at: at(minutes), evidence: 'simulated', capture: snapshot(t, 'second', t.direction === 'inbound' ? 12 : 32, at(minutes)) });
    t.history.push({ at: at(minutes), role: 'operator', action: '完成示例测试称重' });
    return { id, taskId: t.id, plate: t.plate, direction: t.direction, materialId: t.materialId, materialName: t.materialName, partyId: t.partyId, partyName: t.partyName, driver: t.driver, remark: t.remark, materialCode: t.materialCode, materialSpec: t.materialSpec, materialCategoryName: t.materialCategoryName, partyCode: t.partyCode, partyNature: t.partyNature, partyPhone: t.partyPhone, partyAddress: t.partyAddress, carrierId: t.carrierId, carrierName: t.carrierName, carrierCode: t.carrierCode, contactName: t.contactName, contactPhone: t.contactPhone, driverPhone: t.driverPhone, issuedAt: at(minutes), status, gross: 32, tare: 12, net: 20, unit: '吨', records: structuredClone(t.records), history: [{ at: at(minutes), role: 'operator', action: '生成示例测试过磅单' }], printStatus: 'unprinted', reprintCount: 0, reprintAuthorized: false, example: true };
  };
  terminated.history.push({ at: at(60), role: 'supervisor', action: '终止测试任务', reason: '示例：运输取消' });
  const validTicket = ticket(completed, 'TEST-P-001', 'valid', 70);
  const voidTicket = ticket(voided, 'TEST-P-002', 'void', 100);
  voidTicket.history.push({ at: at(60), role: 'supervisor', action: '作废测试单据', reason: '示例：业务信息有误' });
  return {
    version: 3, categories: [{ id: 'category-1', name: '酿酒原料', enabled: true }, { id: 'category-2', name: '副产物', enabled: true }],
    draft: emptyTaskInput(), draftRequestId: `FIRST-${now}-100`, firstRequests: {},
    materials: [{ id: 'material-1', code: 'M0001', name: '酿酒高粱', categoryId: 'category-1', spec: '一级', unit: '吨', enabled: true, remark: '' }, { id: 'material-2', code: 'M0002', name: '酒糟', categoryId: 'category-2', spec: '湿酒糟', unit: '吨', enabled: true, remark: '' }],
    parties: [
      { id: 'party-1', code: 'B0001', name: '示例粮食供应商', shortName: '示例粮商', nature: 'enterprise', roles: ['supplier', 'carrier'], phone: '13800000001', address: '示例地址', creditCode: '', enabled: true, remark: '', contacts: [{ id: 'contact-1', name: '示例联系人', phone: '13800000001', purpose: '业务联系', remark: '', isDefault: true }] },
      { id: 'party-2', code: 'B0002', name: '示例饲料收购方', shortName: '示例客户', nature: 'enterprise', roles: ['customer'], phone: '13800000002', address: '示例地址', creditCode: '', enabled: true, remark: '', contacts: [] },
      { id: 'party-3', code: 'B0003', name: '示例运输合作社', shortName: '示例承运', nature: 'enterprise', roles: ['carrier'], phone: '13800000003', address: '', creditCode: '', enabled: true, remark: '', contacts: [] },
    ],
    tasks: [first, waitingIn, waitingOut, crossDay, completed, terminated, voided], tickets: [validTicket, voidTicket],
    config: { scenePlate: '晋A12345', recognitionCandidate: '晋A12346', recognitionScenario: 'correct', cameraPreset: 'front', role: 'operator', weight: 30, stable: true, timedOut: false, deviceError: false, online: true, evidence: true, submission: 'normal', printOutcome: 'printed' },
    sampleReady: false, owner: null, history: [], sequence: 100,
  };
}
