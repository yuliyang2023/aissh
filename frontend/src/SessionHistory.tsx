import { useState } from 'react';
import { History, Search, X } from 'lucide-react';
import { Host, sessionStatus } from './api';
import { Empty, Modal } from './components';
import { SessionItem, sessionItemHidden } from './session-items';

export function SessionHistory({
  items,
  hosts,
  hiddenItems,
  removedHosts,
  busy,
  onSelect,
  onClose,
}: {
  items: SessionItem[];
  hosts: Pick<Host, 'id' | 'name' | 'address' | 'user' | 'group'>[];
  hiddenItems: Set<string>;
  removedHosts: Set<string>;
  busy: boolean;
  onSelect: (item: SessionItem) => void;
  onClose: () => void;
}) {
  const [search, setSearch] = useState('');
  const [hiddenOnly, setHiddenOnly] = useState(false);
  const query = search.trim().toLowerCase();
  const matches = items
    .map((item) => ({
      item,
      host: hosts.find((host) => host.id === item.hostId),
      hidden: sessionItemHidden(item, hiddenItems) || removedHosts.has(item.hostId),
    }))
    .filter(
      ({ item, host, hidden }) =>
        (!hiddenOnly || hidden) &&
        [item.title, item.preview, host?.name, host?.address, host?.user, host?.group]
          .join(' ')
          .toLowerCase()
          .includes(query),
    )
    .sort((a, b) => (Date.parse(b.item.updatedAt) || 0) - (Date.parse(a.item.updatedAt) || 0));
  return (
    <Modal
      title="会话历史"
      subtitle="查找全部会话，打开后会恢复到左侧列表。"
      className="session-history-modal"
      onClose={onClose}
    >
      <div className="session-history-tools">
        <label className="search">
          <Search size={15} />
          <input
            autoFocus
            aria-label="搜索会话历史"
            placeholder="搜索标题、摘要、服务器…"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
          {search && (
            <button className="icon-button" aria-label="清除历史搜索" onClick={() => setSearch('')}>
              <X size={13} />
            </button>
          )}
        </label>
        <div className="filter-tabs" role="group" aria-label="历史范围">
          <button aria-pressed={!hiddenOnly} onClick={() => setHiddenOnly(false)}>
            全部
          </button>
          <button aria-pressed={hiddenOnly} onClick={() => setHiddenOnly(true)}>
            已隐藏
          </button>
        </div>
        <small>{matches.length} 个会话</small>
      </div>
      <div className="session-history-results">
        {matches.length ? (
          <ul aria-label="历史会话列表">
            {matches.map(({ item, host, hidden }) => (
              <li key={item.key}>
                <button
                  className="session-history-item"
                  aria-label={(hidden ? '恢复并打开 ' : '打开 ') + item.title}
                  disabled={busy}
                  onClick={() => onSelect(item)}
                >
                  <span className="session-history-content">
                    <strong title={item.title}>{item.title}</strong>
                    <small title={host?.address}>
                      {host?.name || item.session?.name || '服务器'} ·{' '}
                      {item.session ? sessionStatus[item.session.status] : '历史会话'}
                    </small>
                    <span className="session-history-preview">{item.preview}</span>
                  </span>
                  <span className="session-history-meta">
                    {item.updatedAt && (
                      <time dateTime={item.updatedAt}>
                        {new Date(item.updatedAt).toLocaleDateString('zh-CN')}
                      </time>
                    )}
                    {hidden && <span className="session-history-hidden">已隐藏</span>}
                    <span className="session-history-open">{hidden ? '恢复并打开' : '打开'}</span>
                  </span>
                </button>
              </li>
            ))}
          </ul>
        ) : (
          <Empty
            icon={<History size={24} />}
            title={query ? '没有匹配的会话' : hiddenOnly ? '没有已隐藏的会话' : '暂无会话历史'}
            description={query ? '试试其他标题、摘要或服务器名称。' : '隐藏的会话会保留在这里。'}
          />
        )}
      </div>
    </Modal>
  );
}
