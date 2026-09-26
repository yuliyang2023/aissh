import { useState } from 'react';
import { Library } from 'lucide-react';
import { api, errorText } from '../api';
import { Modal } from '../components';
export interface SavedWidget {
  key: string;
  hostId: string;
  title: string;
  versions: { version: number; time: string; definition: { id: string; title: string } }[];
}
export function WidgetLibrary({
  sessionId,
  panelId,
  compact = false,
}: {
  sessionId: string;
  panelId?: string;
  compact?: boolean;
}) {
  const [opened, setOpened] = useState(false),
    [items, setItems] = useState<SavedWidget[]>([]),
    [error, setError] = useState(''),
    [busy, setBusy] = useState(false);
  async function open() {
    setOpened(true);
    setError('');
    try {
      setItems(await api.GetSavedWidgets(sessionId));
    } catch (e) {
      setError(errorText(e));
    }
  }
  async function restore(item: SavedWidget, version: number, replace: boolean) {
    setBusy(true);
    setError('');
    try {
      await api.RestoreSavedWidget(sessionId, item.key, version, replace ? panelId || '' : '');
      setOpened(false);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <button
        type="button"
        className={compact ? 'icon-button' : 'button small'}
        aria-label="监控历史组件"
        aria-haspopup="dialog"
        title="监控历史组件"
        onClick={() => void open()}
      >
        {compact ? <Library size={14} /> : '监控历史组件'}
      </button>
      {opened && (
        <Modal
          title="监控历史组件"
          subtitle="当前主机 · 每个组件保留最近 10 个版本"
          onClose={() => setOpened(false)}
        >
          <div className="widget-library">
            {error && <p role="alert">{error}</p>}
            {!items.length && !error && (
              <p>暂无监控历史组件。组件生效后会自动保存，可在这里重新添加或恢复历史版本。</p>
            )}
            {items.map((item) => (
              <article key={item.key}>
                <strong>{item.title}</strong>
                {[...item.versions].reverse().map((v) => (
                  <div key={v.version}>
                    <span>
                      版本 {v.version} · {new Date(v.time).toLocaleString()}
                    </span>
                    <button
                      className="button small"
                      disabled={busy}
                      onClick={() => void restore(item, v.version, false)}
                    >
                      添加为新面板
                    </button>
                    {panelId === v.definition.id && (
                      <button
                        className="button small"
                        disabled={busy}
                        onClick={() => void restore(item, v.version, true)}
                      >
                        应用此版本
                      </button>
                    )}
                  </div>
                ))}
              </article>
            ))}
          </div>
        </Modal>
      )}
    </>
  );
}
