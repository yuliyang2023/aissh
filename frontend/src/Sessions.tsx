import { useEffect, useState } from 'react';
import { ArrowUpRight, Clock3, Plus, RotateCw, TerminalSquare, Trash2 } from 'lucide-react';
import { Catalog, Host, Session, sessionLive, sessionStatus } from './api';
import { Empty } from './components';

const timestamp = (value: string) =>
  new Date(value).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
function elapsed(session: Session, now: number) {
  const seconds = Math.max(
    0,
    Math.floor(
      ((session.endedAt ? Date.parse(session.endedAt) : now) - Date.parse(session.createdAt)) /
        1000,
    ),
  );
  if (seconds < 60) return '不足 1 分钟';
  if (seconds < 3600) return Math.floor(seconds / 60) + ' 分钟';
  return Math.floor(seconds / 3600) + ' 小时 ' + Math.floor((seconds % 3600) / 60) + ' 分钟';
}
export const sessionAddress = (s: Pick<Session, 'user' | 'address' | 'port'>) =>
  (s.user ? s.user + '@' : '') +
  (s.address.includes(':') ? '[' + s.address + ']' : s.address) +
  ':' +
  s.port;

export function Sessions({
  data,
  sessions,
  search,
  busy,
  onOpen,
  onConnect,
  onClose,
  onClearEnded,
  onClearHistory,
  onNew,
}: {
  data: Catalog;
  sessions: Session[];
  search: string;
  busy: boolean;
  onOpen: (session: Session) => void;
  onConnect: (host: Host, previous?: Session) => void;
  onClose: (session: Session) => void;
  onClearEnded: () => void;
  onClearHistory: () => void;
  onNew: () => void;
}) {
  const [tab, setTab] = useState<'current' | 'recent'>('current');
  const [filter, setFilter] = useState('all');
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 15000);
    return () => clearInterval(timer);
  }, []);
  const matches = (...values: string[]) =>
    values.join(' ').toLowerCase().includes(search.toLowerCase());
  const live = sessions.filter(sessionLive).length;
  const ended = sessions.length - live;
  const visible = sessions.filter(
    (s) =>
      matches(s.name, s.address, s.user) &&
      (filter === 'all' || (filter === 'live' ? sessionLive(s) : !sessionLive(s))),
  );
  const recent = (data.recentConnections || []).flatMap((item) => {
    const host = data.hosts.find((h) => h.id === item.hostId);
    return host && matches(host.name, host.address, host.user)
      ? [{ host, openedAt: item.openedAt }]
      : [];
  });
  return (
    <div className="sessions-manager">
      <div className="session-summary">
        <div>
          <span className="status-dot" />
          <strong>{live}</strong>
          <span>运行 / 启动中</span>
        </div>
        <div>
          <span className="status-dot muted" />
          <strong>{ended}</strong>
          <span>已结束</span>
        </div>
        <span>最多同时运行 12 个会话</span>
      </div>
      <div className="sessions-toolbar">
        <div className="filter-tabs" role="group" aria-label="会话范围">
          <button aria-pressed={tab === 'current'} onClick={() => setTab('current')}>
            当前会话
          </button>
          <button aria-pressed={tab === 'recent'} onClick={() => setTab('recent')}>
            最近连接
          </button>
        </div>
        {tab === 'current' ? (
          <>
            <select
              aria-label="会话状态"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
            >
              <option value="all">全部状态</option>
              <option value="live">运行 / 启动中</option>
              <option value="ended">已结束 / 异常</option>
            </select>
            <button className="button small" disabled={busy || !ended} onClick={onClearEnded}>
              <Trash2 size={14} />
              清理已结束
            </button>
          </>
        ) : (
          <button
            className="button small"
            disabled={busy || !data.recentConnections?.length}
            onClick={onClearHistory}
          >
            <Trash2 size={14} />
            清空最近记录
          </button>
        )}
      </div>
      {tab === 'current' ? (
        visible.length ? (
          <div className="table-card">
            <table className="sessions-table">
              <thead>
                <tr>
                  <th>会话 / 目标</th>
                  <th>状态</th>
                  <th>开始时间 / 时长</th>
                  <th className="align-right">操作</th>
                </tr>
              </thead>
              <tbody>
                {visible.map((s) => {
                  const host = data.hosts.find((h) => h.id === s.hostId);
                  return (
                    <tr key={s.id} data-session-id={s.id}>
                      <td>
                        <div className="identity">
                          <span className="row-icon">
                            <TerminalSquare size={18} />
                          </span>
                          <div>
                            <strong title={s.name}>{s.name}</strong>
                            <small className="mono" title={sessionAddress(s)}>
                              {sessionAddress(s)}
                            </small>
                          </div>
                        </div>
                      </td>
                      <td>
                        <span className={'session-state ' + s.status}>
                          <span className={'status-dot ' + s.status} />
                          {sessionStatus[s.status]}
                        </span>
                      </td>
                      <td>
                        <span>{timestamp(s.createdAt)}</span>
                        <small className="cell-small">{elapsed(s, now)}</small>
                      </td>
                      <td>
                        <div className="row-actions">
                          <button className="button small" onClick={() => onOpen(s)}>
                            查看终端
                          </button>
                          {!sessionLive(s) && (
                            <button
                              className="button small connect"
                              disabled={busy || !host}
                              title={!host ? '主机配置已删除' : '使用当前主机配置重新连接'}
                              onClick={() => host && onConnect(host, s)}
                            >
                              <RotateCw size={13} />
                              重连
                            </button>
                          )}
                          <button
                            className="icon-button danger-hover"
                            disabled={busy}
                            aria-label={(sessionLive(s) ? '断开会话 ' : '移除会话 ') + s.name}
                            title={sessionLive(s) ? '断开连接' : '移除已结束的标签页'}
                            onClick={() => onClose(s)}
                          >
                            <Trash2 size={15} />
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty
            icon={<TerminalSquare size={28} />}
            title={sessions.length ? '没有匹配的会话' : '还没有打开的会话'}
            description={
              sessions.length
                ? '调整搜索词或状态筛选后重试。'
                : '连接服务器后，在这里查看状态、切换终端或重新连接。'
            }
            action={
              !sessions.length && (
                <button className="button primary" onClick={onNew}>
                  <Plus size={15} />
                  新建连接
                </button>
              )
            }
          />
        )
      ) : recent.length ? (
        <div className="table-card">
          <table className="sessions-table">
            <thead>
              <tr>
                <th>主机 / 目标</th>
                <th>最近打开</th>
                <th className="align-right">操作</th>
              </tr>
            </thead>
            <tbody>
              {recent.map(({ host, openedAt }) => {
                const active = sessions.find((s) => s.hostId === host.id && sessionLive(s));
                return (
                  <tr key={host.id}>
                    <td>
                      <div className="identity">
                        <span className="row-icon">
                          <Clock3 size={18} />
                        </span>
                        <div>
                          <strong>{host.name}</strong>
                          <small className="mono">{sessionAddress(host)}</small>
                        </div>
                      </div>
                    </td>
                    <td>{timestamp(openedAt)}</td>
                    <td>
                      <div className="row-actions">
                        <button
                          className="button small connect"
                          disabled={busy}
                          onClick={() => (active ? onOpen(active) : onConnect(host))}
                        >
                          {active ? '打开会话' : '重新连接'}
                          <ArrowUpRight size={14} />
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : (
        <Empty
          icon={<Clock3 size={28} />}
          title={search ? '没有匹配的记录' : '还没有最近连接'}
          description="最近打开过的 50 台主机会保存在本机，重启客户端后也能再次连接。"
        />
      )}
      <p className="sessions-note">
        运行中表示终端工作台已启动，SSH
        登录结果请查看终端。退出客户端会结束当前会话；最近记录只保存主机引用和打开时间。
      </p>
    </div>
  );
}
