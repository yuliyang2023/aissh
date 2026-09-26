import { useEffect, useState } from 'react';
import { ArrowUpRight, ChevronRight, MessageSquare, Plus, Server, Sparkles } from 'lucide-react';
import { api, ChatMessage, Conversation, errorText, Host, Session, sessionLive } from './api';
import { Empty, ErrorBox } from './components';
import { sessionAddress } from './Sessions';

export const conversationKey = (c: Pick<Conversation, 'hostId' | 'id'>) => c.hostId + '/' + c.id;
const timestamp = (value: string) =>
  !value || value.startsWith('0001-')
    ? '未知时间'
    : new Date(value).toLocaleString('zh-CN', {
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
      });
const roles: Record<string, string> = {
  user: '你',
  assistant: 'AI 助手',
  proposal: '命令建议',
  system: '系统',
  error: '错误',
};

export function Conversations({
  hosts,
  conversations,
  sessions,
  search,
  busy,
  hostFilter,
  onHostFilter,
  collapsedHosts,
  onToggleHost,
  selected,
  onSelect,
  onNew,
  onContinue,
}: {
  hosts: Host[];
  conversations: Conversation[];
  sessions: Session[];
  search: string;
  busy: boolean;
  hostFilter: string;
  onHostFilter: (id: string) => void;
  collapsedHosts: Set<string>;
  onToggleHost: (id: string) => void;
  selected: string;
  onSelect: (id: string) => void;
  onNew: (hostId?: string) => void;
  onContinue: (conversation: Conversation) => void;
}) {
  const query = search.trim().toLowerCase();
  const groups = hosts
    .filter((h) => !hostFilter || hostFilter === h.id)
    .map((host) => ({
      host,
      items: conversations.filter(
        (c) =>
          c.hostId === host.id &&
          [c.title, c.preview, host.name, host.address, host.user, host.group]
            .join(' ')
            .toLowerCase()
            .includes(query),
      ),
    }))
    .filter(
      ({ host, items }) =>
        items.length ||
        !query ||
        [host.name, host.address, host.user, host.group].join(' ').toLowerCase().includes(query),
    );
  const visible = groups.flatMap((g) => g.items);
  const current = visible.find((c) => conversationKey(c) === selected) || visible[0];
  const currentHost = hosts.find((h) => h.id === current?.hostId);
  const live =
    current &&
    sessions.find(
      (s) => s.hostId === current.hostId && s.conversationId === current.id && sessionLive(s),
    );
  const [detail, setDetail] = useState<{ key: string; messages: ChatMessage[]; error: string }>();
  const key = current ? conversationKey(current) : '';
  useEffect(() => {
    if (!current) return;
    let disposed = false;
    const refresh = () => {
      if (document.hidden) return;
      void api
        .GetConversation(current.hostId, current.id)
        .then((messages) => {
          if (!disposed) setDetail({ key, messages, error: '' });
        })
        .catch((e) => {
          if (!disposed) setDetail({ key, messages: [], error: errorText(e) });
        });
    };
    refresh();
    const interval = setInterval(refresh, 5000);
    return () => {
      disposed = true;
      clearInterval(interval);
    };
  }, [key, current?.updatedAt]);

  if (!hosts.length)
    return (
      <Empty
        icon={<MessageSquare size={30} />}
        title="从一台服务器开始对话"
        description="直接输入服务器地址开始对话，也可以先添加常用主机。"
        action={
          <button className="button primary" onClick={() => onNew()}>
            <Plus size={16} />
            新建对话
          </button>
        }
      />
    );

  return (
    <div className="conversations-manager">
      <div className="conversations-toolbar">
        <select
          aria-label="按服务器筛选 AI 会话"
          value={hostFilter}
          onChange={(e) => onHostFilter(e.target.value)}
        >
          <option value="">全部服务器</option>
          {hosts.map((h) => (
            <option key={h.id} value={h.id}>
              {h.name} · {sessionAddress(h)}
            </option>
          ))}
        </select>
        <span>对话保存在本机，断开 SSH 后仍可查看</span>
      </div>
      <div className="conversation-browser">
        <div className="conversation-list" aria-label="按服务器分组的 AI 会话">
          {groups.map(({ host, items }) => (
            <section key={host.id} className="conversation-group">
              <div className="conversation-group-heading">
                <button
                  className="conversation-group-toggle"
                  aria-label={
                    (collapsedHosts.has(host.id) ? '展开 ' : '收起 ') + host.name + ' 的对话'
                  }
                  aria-expanded={!collapsedHosts.has(host.id)}
                  aria-controls={'host-conversations-' + host.id}
                  onClick={() => onToggleHost(host.id)}
                >
                  <ChevronRight size={14} className="conversation-group-chevron" />
                  <Server size={15} />
                  <span className="conversation-group-name">
                    <strong title={host.name}>{host.name}</strong>
                    <small title={sessionAddress(host)}>{sessionAddress(host)}</small>
                  </span>
                  <span className="nav-count">{items.length}</span>
                </button>
                <button
                  className="icon-button"
                  title={'为 ' + host.name + ' 新建对话'}
                  aria-label={'为 ' + host.name + ' 新建对话'}
                  disabled={busy}
                  onClick={() => onNew(host.id)}
                >
                  <Plus size={15} />
                </button>
              </div>
              <div id={'host-conversations-' + host.id} hidden={collapsedHosts.has(host.id)}>
                {items.map((c) => (
                  <button
                    key={c.id}
                    className={
                      'conversation-item ' + (key === conversationKey(c) ? 'selected' : '')
                    }
                    aria-pressed={key === conversationKey(c)}
                    onClick={() => onSelect(conversationKey(c))}
                  >
                    <strong>
                      <MessageSquare size={14} />
                      <span>{c.title}</span>
                    </strong>
                    <p>{c.preview || '还没有消息，开始新的话题'}</p>
                    <small>
                      <span>{timestamp(c.updatedAt)}</span>
                      <span>{c.messageCount} 条消息</span>
                    </small>
                  </button>
                ))}
                {!items.length && (
                  <p className="conversation-group-empty">
                    {query ? '没有匹配的对话' : '还没有对话'}
                    <button disabled={busy} onClick={() => onNew(host.id)}>
                      新建对话
                    </button>
                  </p>
                )}
              </div>
            </section>
          ))}
          {!groups.length && <p className="conversation-group-empty">没有匹配的服务器或对话</p>}
        </div>
        <div className="conversation-detail">
          {current && currentHost ? (
            <>
              <div className="conversation-detail-heading">
                <div>
                  <small>{currentHost.name} · AI 对话</small>
                  <h2>{current.title}</h2>
                </div>
                <button
                  className="button primary"
                  disabled={busy}
                  onClick={() => onContinue(current)}
                >
                  {live ? '返回对话' : current.messageCount ? '继续对话' : '开始对话'}
                  <ArrowUpRight size={15} />
                </button>
              </div>
              <div className="conversation-messages" key={key}>
                {detail?.key !== key ? (
                  <p className="subtle">正在读取对话…</p>
                ) : detail.error ? (
                  <ErrorBox error={detail.error} />
                ) : current.messageCount === 0 ? (
                  <Empty
                    icon={<Sparkles size={28} />}
                    title="开始一个新话题"
                    description={
                      '点击“开始对话”连接 ' + currentHost.name + '，向 AI 描述你要处理的问题。'
                    }
                  />
                ) : (
                  detail.messages.map((m, i) => (
                    <article key={i} className={'chat-message ' + m.role}>
                      <header>
                        <strong>{roles[m.role] || '系统'}</strong>
                        <time>{timestamp(m.time)}</time>
                      </header>
                      <div className="chat-content">{m.content}</div>
                      {m.proposal && (
                        <div className="chat-proposal">
                          <small>历史命令建议 · {m.proposal.risk}</small>
                          <pre>{m.proposal.command}</pre>
                        </div>
                      )}
                    </article>
                  ))
                )}
              </div>
              <div className="conversation-detail-footer">
                <Sparkles size={15} />
                <span>继续对话会恢复这段上下文，并打开服务器工作台。</span>
              </div>
            </>
          ) : (
            <Empty
              icon={<MessageSquare size={30} />}
              title={query ? '没有匹配的对话' : '选择或新建一段对话'}
              description={
                query
                  ? '试试其他关键词，或切换服务器。'
                  : '每台服务器可以保留多段对话，分别处理不同的问题。'
              }
              action={
                !query && (
                  <button
                    className="button primary"
                    disabled={busy}
                    onClick={() => onNew(hostFilter)}
                  >
                    <Plus size={15} />
                    新建对话
                  </button>
                )
              }
            />
          )}
        </div>
      </div>
    </div>
  );
}
