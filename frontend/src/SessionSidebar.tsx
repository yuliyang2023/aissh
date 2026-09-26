import { RefObject } from 'react';
import { ChevronRight, MessageSquare, Plus, Search, Server, TerminalSquare, X } from 'lucide-react';
import { Conversation, Host, Session, sessionLive, sessionStatus } from './api';
import { conversationKey } from './Conversations';
import { sessionAddress } from './api';

export function conversationSession(sessions: Session[], c: Conversation) {
  const matches = sessions.filter((s) => s.hostId === c.hostId && s.conversationId === c.id);
  return matches.find(sessionLive) || matches[matches.length - 1];
}

export function SessionSidebar({
  hosts,
  conversations,
  sessions,
  selectedConversation,
  activeSession,
  search,
  onSearch,
  searchRef,
  collapsedHosts,
  onToggleHost,
  busy,
  onSelect,
  onOpen,
  onClose,
  onNew,
}: {
  hosts: Host[];
  conversations: Conversation[];
  sessions: Session[];
  selectedConversation: string;
  activeSession: string;
  search: string;
  onSearch: (value: string) => void;
  searchRef: RefObject<HTMLInputElement | null>;
  collapsedHosts: Set<string>;
  onToggleHost: (id: string) => void;
  busy: boolean;
  onSelect: (conversation: Conversation) => void;
  onOpen: (session: Session) => void;
  onClose: (session: Session) => void;
  onNew: (hostId?: string) => void;
}) {
  const query = search.trim().toLowerCase();
  // Keep running connections reachable even if their saved host was removed in Settings.
  const groupHosts = [
    ...hosts,
    ...sessions
      .filter(
        (s, index) =>
          !hosts.some((h) => h.id === s.hostId) &&
          !sessions.slice(0, index).some((other) => other.hostId === s.hostId),
      )
      .map((s) => ({
        id: s.hostId,
        name: s.name,
        address: s.address,
        port: s.port,
        user: s.user,
        group: '',
      })),
  ];
  const groups = groupHosts
    .map((host) => {
      const items: {
        key: string;
        title: string;
        preview: string;
        conversation?: Conversation;
        session?: Session;
      }[] = conversations
        .filter((c) => c.hostId === host.id)
        .map((c) => ({
          key: conversationKey(c),
          title: c.title,
          preview: c.preview || '新的话题',
          conversation: c,
          session: conversationSession(sessions, c),
        }));
      sessions
        .filter(
          (s) =>
            s.hostId === host.id &&
            !conversations.some((c) => c.hostId === s.hostId && c.id === s.conversationId),
        )
        .forEach((s) =>
          items.unshift({
            key: 'session:' + s.id,
            title: s.name,
            preview: 'SSH + AI 工作区',
            session: s,
          }),
        );
      return {
        host,
        items: items.filter((item) =>
          [item.title, item.preview, host.name, host.address, host.user, host.group]
            .join(' ')
            .toLowerCase()
            .includes(query),
        ),
      };
    })
    .filter((group) => group.items.length);
  return (
    <>
      <div className="session-sidebar-tools">
        <button className="button primary" disabled={busy} onClick={() => onNew()}>
          <Plus size={16} />
          新建会话
        </button>
        <label className="search">
          <Search size={14} />
          <input
            ref={searchRef}
            aria-label="搜索会话"
            placeholder="搜索对话、服务器…"
            value={search}
            onChange={(e) => onSearch(e.target.value)}
          />
          {search && (
            <button className="icon-button" aria-label="清除会话搜索" onClick={() => onSearch('')}>
              <X size={12} />
            </button>
          )}
        </label>
      </div>
      <div className="unified-session-list" aria-label="SSH 与 AI 会话列表">
        {groups.map(({ host, items }) => (
          <section className="session-host-group" key={host.id}>
            <div className="session-host-heading">
              <button
                aria-label={
                  (collapsedHosts.has(host.id) && !query ? '展开 ' : '收起 ') +
                  host.name +
                  ' 的会话'
                }
                aria-expanded={!!query || !collapsedHosts.has(host.id)}
                onClick={() => onToggleHost(host.id)}
              >
                <ChevronRight size={13} />
                <Server size={13} />
                <span title={sessionAddress(host)}>{host.name}</span>
                <small>{items.length}</small>
              </button>
              <button
                className="icon-button"
                aria-label={'为 ' + host.name + ' 新建会话'}
                disabled={busy}
                onClick={() => onNew(host.id)}
              >
                <Plus size={13} />
              </button>
            </div>
            <div hidden={!query && collapsedHosts.has(host.id)}>
              {items.map((item) => {
                const selected = activeSession
                  ? item.session?.id === activeSession
                  : item.key === selectedConversation;
                return (
                  <div
                    className={'unified-session-row ' + (selected ? 'selected' : '')}
                    key={item.key}
                  >
                    <button
                      className="session-select"
                      aria-pressed={selected}
                      disabled={busy}
                      onClick={() =>
                        item.conversation ? onSelect(item.conversation) : onOpen(item.session!)
                      }
                    >
                      {item.session ? (
                        <span className={'status-dot ' + item.session.status} />
                      ) : (
                        <MessageSquare size={13} />
                      )}
                      <span>
                        <strong>{item.title}</strong>
                        <small>{item.preview}</small>
                        <em>{item.session ? sessionStatus[item.session.status] : '历史会话'}</em>
                      </span>
                    </button>
                    {item.session && (
                      <button
                        className="icon-button session-close"
                        title={sessionLive(item.session) ? '断开连接' : '移除已结束的连接'}
                        aria-label={(sessionLive(item.session) ? '断开 ' : '移除 ') + item.title}
                        disabled={busy}
                        onClick={() => onClose(item.session!)}
                      >
                        <X size={13} />
                      </button>
                    )}
                  </div>
                );
              })}
            </div>
          </section>
        ))}
        {!groups.length && (
          <div className="sidebar-empty">
            <TerminalSquare size={22} />
            <p>{query ? '没有匹配的会话' : '从一个新会话开始'}</p>
            <small>{query ? '试试其他关键词' : '连接服务器后，对话和 SSH 会一起保存在这里'}</small>
          </div>
        )}
      </div>
    </>
  );
}
