import { useEffect, useState } from 'react';
import type { MonitorPanel } from './api';
import { Modal } from './components';
import { ShellCode } from './ShellCode';

export function CollectorConfig({ panel, onClose }: { panel: MonitorPanel; onClose: () => void }) {
  const command = panel.command || '';
  const [formatted, setFormatted] = useState('');
  const [original, setOriginal] = useState(false);
  const [status, setStatus] = useState('正在格式化…');

  useEffect(() => {
    if (!command) return;
    if (command.length > 256 * 1024) {
      setStatus('命令较长，显示原始内容');
      return;
    }
    let cancelled = false;
    // Load the formatter only when this rarely used dialog is opened.
    void import('./formatCollectorCommand')
      .then(({ formatCollectorCommand }) => formatCollectorCommand(command))
      .then((text) => {
        if (cancelled) return;
        setFormatted(text);
        setStatus('已格式化');
      })
      .catch(() => {
        if (!cancelled) setStatus('暂无法格式化，显示原始命令');
      });
    return () => {
      cancelled = true;
    };
  }, [command]);

  return (
    <Modal
      title="采集配置"
      subtitle={`${panel.title} · 每 ${Math.max(1, panel.intervalMs / 1000)} 秒刷新 · 仅展示数据`}
      className="collector-config"
      onClose={onClose}
    >
      <div className="collector-config-body">
        {command ? (
          <>
            <div className="collector-config-toolbar">
              <strong>采集命令</strong>
              <span className="command-language">Shell</span>
              <small role="status">{original ? '原始命令' : status}</small>
              {formatted && (
                <button
                  type="button"
                  className="button small"
                  onClick={() => setOriginal(!original)}
                >
                  {original ? '查看格式化' : '查看原文'}
                </button>
              )}
            </div>
            <ShellCode command={original ? command : formatted || command} wrap />
          </>
        ) : (
          <p className="collector-config-empty">尚未收到采集命令</p>
        )}
      </div>
    </Modal>
  );
}
