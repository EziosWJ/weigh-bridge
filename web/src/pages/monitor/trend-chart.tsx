export type TrendPoint = { time: string; values: Record<string, number | null>; gap?: boolean };
export type TrendSeries = { key: string; label: string; color: string };

type TrendChartProps = { points: TrendPoint[]; series: TrendSeries[]; unit: string };

export function TrendChart({ points, series, unit }: TrendChartProps) {
  const times = points.map((point) => Date.parse(point.time));
  const first = times[0] ?? 0;
  const last = times[times.length - 1] ?? first;
  const maximum = Math.max(1, ...points.flatMap((point) => series.map(({ key }) => point.values[key] ?? 0)));
  const x = (index: number) => 50 + ((times[index] - first) / Math.max(5000, last - first)) * 690;
  const y = (value: number) => 175 - (value / maximum) * 145;
  const paths = (key: string) => {
    const segments: string[] = [];
    let segment = "";
    points.forEach((point, index) => {
      const value = point.values[key];
      if (point.gap || value == null || (index > 0 && times[index] - times[index - 1] > 15000)) {
        if (segment) segments.push(segment);
        segment = "";
      }
      if (!point.gap && value != null) segment += `${segment ? " L" : "M"}${x(index).toFixed(1)},${y(value).toFixed(1)}`;
    });
    if (segment) segments.push(segment);
    return segments;
  };
  return <div>
    <div className="mb-2 flex flex-wrap gap-4 text-xs text-text-secondary">{series.map((item) => <span key={item.key} className="flex items-center gap-2"><span style={{ backgroundColor: item.color }} className="h-2 w-2 rounded-full" />{item.label}（{unit}）</span>)}</div>
    {points.length === 0 ? <p className="py-12 text-center text-sm text-text-tertiary">采集中，暂无历史样本</p> : <>
      <svg viewBox="0 0 780 210" role="img" aria-label="最近30分钟资源趋势，采集缺口不连接" className="w-full">
        {[0, 0.5, 1].map((fraction) => <g key={fraction}><line x1="50" x2="740" y1={y(maximum * fraction)} y2={y(maximum * fraction)} stroke="#e2e8f0" /><text x="44" y={y(maximum * fraction) + 4} textAnchor="end" fontSize="11" fill="#64748b">{(maximum * fraction).toFixed(1)}</text></g>)}
        {series.flatMap((item) => paths(item.key).map((path, index) => <path key={`${item.key}-${index}`} data-series={item.key} data-segment={index} d={path} fill="none" stroke={item.color} strokeWidth="2" />))}
        {series.flatMap((item) => points.map((point, index) => !point.gap && point.values[item.key] != null ? <circle key={`${item.key}-${index}`} cx={x(index)} cy={y(point.values[item.key]!)} r="2" fill={item.color}><title>{new Date(point.time).toLocaleTimeString()} {item.label}: {point.values[item.key]?.toFixed(2)} {unit}</title></circle> : null))}
        <text x="50" y="199" fontSize="11" fill="#64748b">{new Date(first).toLocaleTimeString()}</text><text x="740" y="199" textAnchor="end" fontSize="11" fill="#64748b">{new Date(last).toLocaleTimeString()}</text>
      </svg>
      <p className="text-xs text-text-tertiary">实际历史 {points.length} 个样本；最多保留30分钟。空值和采集中断形成缺口，API 重启后重新积累。</p>
    </>}
  </div>;
}
