import { useState } from 'react';
import { Activity, Check, Copy, Search } from 'lucide-react';
import { errorText } from './api';
import { MonitoringState, MonitorSummary, conditionSummary } from './monitoring-api';
import { ShellCode } from './ShellCode';

const statuses: Record<MonitorSummary['status'], string> = {
  watching: '等待下一轮采集',
  collecting: '正在采集',
  waiting: '等待首次采集',
  unknown: '数据无法判定',
  error: '采集失败',
  stale: '数据已过期',
  stopped: '连接已结束',
};
const origins: Record<string, string> = { ai: 'AI 创建', user: '手动添加', system: '系统默认' };

function alertSummary(m: MonitorSummary, state: MonitoringState) {
  if (!state.policy.enabled) return '异常提醒已全局关闭，仍然采集数据';
  if (m.alert) return conditionSummary(m.alert);
  return '未收到此任务的异常条件';
}

export function MonitorInventory({
  state,
  busy,
  onOpenSession,
  onAction,
  scoped = false,
}: {
  state: MonitoringState;
  busy: boolean;
  onOpenSession: (id: string) => void;
  onAction: (id: string, panelId: string, kind: 'alert-refresh' | 'alert-close') => void;
  scoped?: boolean;
}) {
  const [query, setQuery] = useState('');
  const [session, setSession] = useState('');
  const [copied, setCopied] = useState('');
  const [error, setError] = useState('');
  const monitors = state.monitors || [];
  const sessions = [...new Map(monitors.map((m) => [m.sessionId, m.sessionName])).entries()];
  const filter = sessions.some(([id]) => id === session) ? session : '';
  const visible = monitors.filter(
    (m) =>
      (!filter || m.sessionId === filter) &&
      `${m.title} ${m.sessionName} ${m.command || ''}`
        .toLowerCase()
        .includes(query.trim().toLowerCase()),
  );
  const running = monitors.filter((m) => m.status !== 'stopped').length;
  return (
    <section className="monitor-inventory" aria-label="告警任务清单">
      <div className="monitor-inventory-summary">
        <strong>{monitors.length} 个告警任务</strong>
        <span>
          {running} 个随连接运行 · {monitors.length - running} 个连接已结束
        </span>
      </div>
      <div className="monitor-inventory-filters">
        <label className="search">
          <Search size={14} />
          <input
            aria-label="搜索告警任务"
            placeholder="搜索任务、服务器或命令…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
        {!scoped && (
          <select
            aria-label="筛选监控会话"
            value={filter}
            onChange={(e) => setSession(e.target.value)}
          >
            <option value="">全部会话</option>
            {sessions.map(([id, name]) => (
              <option key={id} value={id}>
                {name}
              </option>
            ))}
          </select>
        )}
      </div>
      {error && (
        <p className="error-box" role="alert">
          {error}
        </p>
      )}
      {!visible.length ? (
        <div className="alert-empty">
          <Activity size={24} />
          <strong>
            {monitors.length
              ? '没有匹配的告警任务'
              : scoped
                ? '当前会话还没有告警任务'
                : '还没有告警任务'}
          </strong>
          <p>
            {monitors.length
              ? '调整搜索条件或切换会话查看。'
              : '连接服务器后，通过「AI 添加告警任务」描述要检查什么、多久检查和何时提醒。实时面板中的项目不会自动加入。'}
          </p>
        </div>
      ) : (
        <div className="monitor-inventory-list">
          {visible.map((m) => {
            const key = m.sessionId + ':' + m.panelId;
            const updated =
              !!m.updatedAt &&
              !m.updatedAt.startsWith('0001-') &&
              Number.isFinite(Date.parse(m.updatedAt));
            return (
              <article
                key={key}
                className="monitor-inventory-item"
                aria-label={`${m.sessionName} · ${m.title}`}
              >
                <header>
                  <div>
                    <strong>{m.title}</strong>
                    <span className="monitor-origin">{origins[m.origin] || '已有项目'}</span>
                  </div>
                  <span className={'monitor-collection-status ' + m.status}>
                    {statuses[m.status] || '等待更新'}
                  </span>
                </header>
                <dl className="monitor-inventory-meta">
                  <div>
                    <dt>所在会话</dt>
                    <dd>{m.sessionName}</dd>
                  </div>
                  <div>
                    <dt>检查周期</dt>
                    <dd>每 {m.intervalMs / 1000} 秒</dd>
                  </div>
                  <div>
                    <dt>最近采集</dt>
                    <dd>
                      {updated
                        ? new Date(m.updatedAt).toLocaleString('zh-CN', { hour12: false })
                        : '尚未采集'}
                    </dd>
                  </div>
                </dl>
                <div className="monitor-command-heading">
                  <strong>采集命令</strong>
                  {m.command && (
                    <button
                      className="button small"
                      onClick={async () => {
                        setError('');
                        try {
                          await navigator.clipboard.writeText(m.command);
                          setCopied(key);
                        } catch (e) {
                          setError('复制失败，可选中下方命令手动复制：' + errorText(e));
                        }
                      }}
                    >
                      {copied === key ? <Check size={12} /> : <Copy size={12} />}
                      {copied === key ? '已复制' : '复制命令'}
                    </button>
                  )}
                </div>
                <div className="monitor-inventory-command">
                  {m.command ? (
                    <ShellCode command={m.command} wrap />
                  ) : (
                    <p>尚未收到采集命令；旧版本会话需重新连接后读取。</p>
                  )}
                </div>
                {m.error && (
                  <p className="monitor-inventory-error" role="status">
                    最近采集失败：{m.error}
                  </p>
                )}
                <div className="monitor-inventory-rule">
                  <span>异常提醒</span>
                  <p>{alertSummary(m, state)}</p>
                  {m.alert?.enabled && <code>{m.alert.pattern}</code>}
                </div>
                <footer>
                  <span>
                    {m.status === 'stopped'
                      ? '保留上次配置，当前不再采集'
                      : '检查周期为计划间隔，SSH 忙碌时可能延后'}
                  </span>
                  <button
                    className="button small"
                    disabled={busy || m.status === 'stopped'}
                    onClick={() => onAction(m.sessionId, m.panelId, 'alert-refresh')}
                  >
                    立即检查
                  </button>
                  <button
                    className="button small"
                    disabled={busy || m.status === 'stopped'}
                    onClick={() => onAction(m.sessionId, m.panelId, 'alert-close')}
                  >
                    停止并移除
                  </button>
                  <button
                    className="button small"
                    disabled={busy}
                    onClick={() => onOpenSession(m.sessionId)}
                  >
                    查看会话
                  </button>
                </footer>
              </article>
            );
          })}
        </div>
      )}
      <p className="monitoring-note">
        这里只显示独立告警任务，桌宠接收这些任务的异常与恢复提醒。采集失败、数据过期与断线沿用提醒策略；实时面板单独管理。
      </p>
    </section>
  );
}
