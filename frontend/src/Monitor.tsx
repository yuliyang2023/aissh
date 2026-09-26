import { useId, useState } from 'react';
import { Activity, ChevronDown, RefreshCw, X } from 'lucide-react';
import { MonitorPanel, WorkbenchCommand } from './api';
import { useStoredFlag } from './layout';

export function overviewValues(content = '') {
  return Object.fromEntries(
    content.split(/\r?\n/).flatMap((line) => {
      const match = line.match(/^(HOST|UPTIME|LOAD|MEM|DISK|PROCS)=(.*)$/);
      return match ? [[match[1], match[2].trim()]] : [];
    }),
  );
}
const updated = (panel?: MonitorPanel) =>
  !!panel?.updatedAt && !panel.updatedAt.startsWith('0001-');
const stalePanel = (panel: MonitorPanel, now: number) =>
  updated(panel) && now - Date.parse(panel.updatedAt) > Math.max(15000, panel.intervalMs * 3);

function Metric({ label, value, percent }: { label: string; value?: string; percent?: boolean }) {
  const number = percent ? Number(value?.match(/\(([\d.]+)%\)/)?.[1]) : NaN;
  return (
    <div className={'monitor-metric ' + (number >= 90 ? 'warning' : '')}>
      <small>{label}</small>
      <strong title={value}>{value || '—'}</strong>
      {Number.isFinite(number) && (
        <div className="metric-track">
          <span style={{ width: Math.min(100, Math.max(0, number)) + '%' }} />
        </div>
      )}
    </div>
  );
}

export function Monitor({
  panels,
  online,
  pending,
  now,
  onAction,
}: {
  panels: MonitorPanel[];
  online: boolean;
  pending: boolean;
  now: number;
  onAction: (command: WorkbenchCommand) => Promise<boolean>;
}) {
  const detailsId = useId();
  const [expanded, setExpanded] = useStoredFlag('aissh.monitor.expanded', true);
  const [selected, setSelected] = useState('system');
  const [custom, setCustom] = useState(false);
  const [command, setCommand] = useState('');
  const current = panels.find((p) => p.id === selected) || panels[0];
  const overview = panels.find((p) => p.id === 'system');
  const metrics = overviewValues(overview?.content);
  const failed = panels.some((p) => p.error);
  const stale = panels.some((p) => stalePanel(p, now));
  const status = !online
    ? '未连接'
    : failed
      ? '采集异常'
      : stale
        ? '数据过期'
        : !updated(overview)
          ? '等待采集'
          : '实时';
  return (
    <section className="native-monitor" aria-label="服务器监控">
      <button
        className="monitor-heading"
        title={expanded ? '收起服务器监控' : '展开服务器监控'}
        aria-expanded={expanded}
        aria-controls={detailsId}
        onClick={() => setExpanded(!expanded)}
      >
        <Activity size={16} />
        <strong>服务器监控</strong>
        <span className={'monitor-health ' + (failed || stale || !online ? 'warning' : '')}>
          {status}
        </span>
        {!expanded && (
          <span className="monitor-summary">
            负载 {metrics.LOAD || '—'} · 内存 {metrics.MEM || '—'}
          </span>
        )}
        <ChevronDown size={15} className={expanded ? '' : 'folded'} />
      </button>
      {expanded && (
        <div id={detailsId} className="monitor-details">
          <div className="monitor-tabs">
            <div role="group" aria-label="监控项目">
              {panels.map((p) => (
                <button
                  key={p.id}
                  aria-pressed={current?.id === p.id}
                  onClick={() => setSelected(p.id)}
                >
                  {p.title}
                  {p.error && ' !'}
                </button>
              ))}
            </div>
            <select
              aria-label="添加监控"
              disabled={!online || pending}
              value=""
              onChange={async (e) => {
                const kind = e.target.value;
                if (kind === 'custom') {
                  setCustom(true);
                  return;
                }
                if (kind && (await onAction({ kind: 'monitor-open', panelId: kind })))
                  setSelected('desktop-' + kind);
              }}
            >
              <option value="">+ 添加监控</option>
              {[
                ['processes', '进程'],
                ['disk', '磁盘'],
                ['network', '网络'],
                ['services', '服务'],
                ['custom', '自定义'],
              ].map(([id, title]) => (
                <option key={id} value={id}>
                  {title}
                </option>
              ))}
            </select>
          </div>
          {custom && (
            <form
              className="monitor-custom"
              onSubmit={async (e) => {
                e.preventDefault();
                if (await onAction({ kind: 'monitor-open', panelId: 'custom', text: command })) {
                  setCustom(false);
                  setCommand('');
                }
              }}
            >
              <input
                aria-label="只读监控命令"
                placeholder="输入只读监控命令，如 tail -n 30 /var/log/nginx/error.log"
                value={command}
                onChange={(e) => setCommand(e.target.value)}
                required
              />
              <button className="button small" disabled={!online || pending}>
                添加
              </button>
              <button
                type="button"
                className="icon-button"
                aria-label="取消添加监控"
                onClick={() => setCustom(false)}
              >
                <X size={14} />
              </button>
            </form>
          )}
          {current?.error && (
            <p className="monitor-warning" role="status">
              采集失败：{current.error} {current.content && '· 下方保留上次数据'}
            </p>
          )}
          {current && !current.error && (stalePanel(current, now) || !online) && (
            <p className="monitor-warning" role="status">
              数据已停止更新，下方为上次采集结果。
            </p>
          )}
          {!current || current.id === 'system' ? (
            <div className="monitor-metrics">
              <Metric label="系统负载 · 1 / 5 / 15 分钟" value={metrics.LOAD} />
              <Metric label="内存" value={metrics.MEM} percent />
              <Metric label="根目录磁盘" value={metrics.DISK} percent />
              <Metric label="进程数" value={metrics.PROCS} />
            </div>
          ) : (
            <pre className="monitor-output">{current.content || '正在等待采集结果…'}</pre>
          )}
          <div className="monitor-footer">
            <span title={current?.id === 'system' ? metrics.UPTIME : current?.title}>
              {current?.id === 'system' ? metrics.HOST || '等待服务器响应' : current?.title}
            </span>
            <time>
              {updated(current)
                ? new Date(current!.updatedAt).toLocaleTimeString('zh-CN', { hour12: false }) +
                  ' 更新'
                : '尚无数据'}
            </time>
            {current && (
              <>
                <button
                  className="icon-button"
                  aria-label="刷新当前监控"
                  disabled={!online || pending}
                  onClick={() => void onAction({ kind: 'monitor-refresh', panelId: current.id })}
                >
                  <RefreshCw size={13} />
                </button>
                {!current.pinned && (
                  <button
                    className="icon-button"
                    aria-label={'关闭监控 ' + current.title}
                    disabled={!online || pending}
                    onClick={() => void onAction({ kind: 'monitor-close', panelId: current.id })}
                  >
                    <X size={13} />
                  </button>
                )}
              </>
            )}
          </div>
        </div>
      )}
    </section>
  );
}
