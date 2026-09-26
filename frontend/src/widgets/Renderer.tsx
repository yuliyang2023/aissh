import { createElement, useState } from 'react';
import type { ViewNode } from './protocol';

type Point = { time: string; value: number | null };
function Chart({
  series = [],
  unit = '',
  label = '',
  bar = false,
}: {
  series?: Point[];
  unit?: string;
  label?: string;
  bar?: boolean;
}) {
  const points = series.filter((p) => p.value !== null);
  const nums = points.map((p) => p.value!);
  const min = Math.min(0, ...nums),
    max = Math.max(1, ...nums),
    range = max - min || 1;
  const from = Date.parse(series[0]?.time || '') || 0,
    to = Date.parse(series.at(-1)?.time || '') || from + 1;
  const x = (p: Point) => 36 + ((Date.parse(p.time) - from) / (to - from || 1)) * 548;
  const y = (p: Point) => 132 - ((p.value! - min) / range) * 112;
  const paths: string[] = [];
  let path = '';
  for (const p of series) {
    if (p.value === null) {
      if (path) paths.push(path);
      path = '';
    } else path += (path ? ' L' : 'M') + x(p).toFixed(2) + ' ' + y(p).toFixed(2);
  }
  if (path) paths.push(path);
  return (
    <figure className="widget-chart">
      <figcaption>
        {label || '历史趋势'} <small>{unit}</small>
      </figcaption>
      {!points.length ? (
        <p className="widget-empty">等待有效采样…</p>
      ) : (
        <svg viewBox="0 0 600 164" role="img" aria-label={label || '历史趋势'}>
          <line x1="36" x2="584" y1="132" y2="132" stroke="currentColor" opacity=".12" />
          <text x="4" y="25" className="widget-axis">
            {max.toLocaleString(undefined, { maximumFractionDigits: 1 })}
          </text>
          <text x="4" y="132" className="widget-axis">
            {min.toLocaleString(undefined, { maximumFractionDigits: 1 })}
          </text>
          {bar
            ? points.map((p, i) => (
                <rect
                  key={i}
                  x={x(p) - 2}
                  y={y(p)}
                  width={Math.max(1, Math.min(14, 500 / series.length))}
                  height={Math.max(1, 132 - y(p))}
                  rx="1"
                  fill="var(--widget-accent)"
                />
              ))
            : paths.map((d, i) => (
                <path key={i} d={d} fill="none" stroke="var(--widget-accent)" strokeWidth="2.5" />
              ))}
          {points.length === 1 && (
            <circle cx={x(points[0])} cy={y(points[0])} r="3" fill="var(--widget-accent)" />
          )}
          <text x="36" y="156" className="widget-axis">
            {new Date(from).toLocaleTimeString()}
          </text>
          <text x="584" y="156" textAnchor="end" className="widget-axis">
            {new Date(to).toLocaleTimeString()}
          </text>
        </svg>
      )}
    </figure>
  );
}
function DataTable({
  columns = [],
  rows = [],
}: {
  columns?: string[];
  rows?: Record<string, unknown>[];
}) {
  const [page, setPage] = useState(0);
  const pages = Math.max(1, Math.ceil(rows.length / 25));
  const current = Math.min(page, pages - 1);
  return (
    <div className="widget-table">
      <div className="widget-table-scroll">
        <table>
          <thead>
            <tr>
              {columns.map((c) => (
                <th key={c}>{c}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.slice(current * 25, (current + 1) * 25).map((r, i) => (
              <tr key={i}>
                {columns.map((c) => (
                  <td key={c}>{r[c] == null ? '—' : String(r[c])}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="widget-pagination">
        <span>
          {rows.length} 条 · 第 {current + 1}/{pages} 页
        </span>
        <button type="button" disabled={!current} onClick={() => setPage(current - 1)}>
          上一页
        </button>
        <button type="button" disabled={current >= pages - 1} onClick={() => setPage(current + 1)}>
          下一页
        </button>
      </div>
    </div>
  );
}
export function WidgetRenderer({
  tree,
  onEvent,
}: {
  tree: ViewNode[];
  onEvent: (id: string, payload: unknown) => void;
}) {
  function render(n: ViewNode, key: string): any {
    if (n.kind === 'text') return n.text;
    const p = { ...n.props };
    if (n.type === 'MetricCard')
      return (
        <article key={key} className={'widget-metric ' + (p.tone || 'normal')} style={p.style}>
          <small>{p.label}</small>
          <strong>
            {p.value == null ? '—' : p.value}
            <span>{p.unit}</span>
          </strong>
          {typeof p.percent === 'number' && (
            <div className="widget-meter">
              <i style={{ width: Math.max(0, Math.min(100, p.percent)) + '%' }} />
            </div>
          )}
        </article>
      );
    if (n.type === 'StatusBadge')
      return (
        <span key={key} style={p.style} className={'widget-badge ' + (p.tone || 'normal')}>
          {p.label}
          {p.value != null && ' · ' + p.value}
        </span>
      );
    if (n.type === 'LineChart' || n.type === 'BarChart')
      return <Chart key={key} {...p} bar={n.type === 'BarChart'} />;
    if (n.type === 'DataTable') return <DataTable key={key} {...p} />;
    const handlers: Record<string, any> = {};
    for (const [event, id] of Object.entries(n.events))
      handlers[event] = (e: any) =>
        onEvent(id, {
          value: String(e.currentTarget.value ?? '').slice(0, 512),
          checked: !!e.currentTarget.checked,
        });
    if (n.type === 'button') p.type = 'button';
    if (n.type === 'input') p.maxLength = Math.min(p.maxLength ?? 512, 512);
    const children = n.children.map((c, i) => render(c, key + '.' + i));
    return createElement(
      n.type,
      { ...p, ...handlers, key },
      ...(n.type === 'input' ? [] : children),
    );
  }
  return <div className="widget-canvas">{tree.map((n, i) => render(n, String(i)))}</div>;
}
