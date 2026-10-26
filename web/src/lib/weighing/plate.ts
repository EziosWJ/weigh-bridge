export const provinces = [...'晋京津冀蒙辽吉黑沪苏浙皖闽赣鲁豫鄂湘粤桂琼渝川贵云藏陕甘青宁新'];
export const normalizePlate = (value: string) => value.replace(/[\s·•]/g, '').toUpperCase();
export const PLATE_MAX_LENGTH = 8;

/** 普通车牌 7 位、新能源 8 位；输入阶段就按完整车牌截断。 */
export function limitPlate(value: string, province = '') {
  const normalized = normalizePlate(value);
  if (provinces.includes(normalized[0])) return normalized.slice(0, PLATE_MAX_LENGTH);
  return province + normalized.slice(0, PLATE_MAX_LENGTH - 1);
}

/** 新录入覆盖普通及新能源号牌；旧特殊测试值由原记录显式兼容。 */
export function plateError(value: string) {
  const plate = normalizePlate(value);
  if (!provinces.includes(plate[0])) return '请选择车牌省份简称';
  if (!/^[A-HJ-NP-Z]$/.test(plate[1] ?? '')) return '省份后首位须为字母（不含 I、O）';
  const tail = plate.slice(2);
  if (/^[A-HJ-NP-Z\d]{5}$/.test(tail)) return '';
  if (/^(?:[DF][A-HJ-NP-Z\d]\d{4}|\d{5}[DF])$/.test(tail)) return '';
  return '普通车牌共7位；新能源共8位，D/F 位于号码首位或末位，不使用 I、O';
}

/** 现场、识别与提交场景变化使采集失效；岗位切换仅实时限制操作。 */
