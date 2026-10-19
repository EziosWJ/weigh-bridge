/** 业务日期始终按北京时间，与浏览器本地时区无关。 */
export function beijingDate(value: string): string {
  return new Date(new Date(value).getTime() + 8 * 3600_000).toISOString().slice(0, 10);
}
export function beijingTime(value: string): string {
  return new Date(new Date(value).getTime() + 8 * 3600_000).toISOString().slice(0, 19).replace('T', ' ');
}
