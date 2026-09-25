import { useEffect, useRef, useState } from 'react';
import { Terminal as XTerminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { api, errorText, Session, sessionLive, sessionStatus } from './api';

export function Terminal({
  session,
  active,
  onReconnect,
  canReconnect,
}: {
  session: Session;
  active: boolean;
  onReconnect: () => void;
  canReconnect: boolean;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const fit = useRef<FitAddon | null>(null);
  const terminal = useRef<XTerminal | null>(null);
  const [error, setError] = useState('');
  const live = useRef(sessionLive(session));
  live.current = sessionLive(session);
  useEffect(() => {
    const term = new XTerminal({
      fontFamily: '"SFMono-Regular", Consolas, "Liberation Mono", monospace',
      fontSize: 13,
      lineHeight: 1.18,
      cursorBlink: true,
      scrollback: 20000,
      theme: {
        background: '#101923',
        foreground: '#dce4ed',
        cursor: '#65d6b1',
        selectionBackground: '#315c6b',
      },
    });
    const addon = new FitAddon();
    term.loadAddon(addon);
    term.open(ref.current!);
    terminal.current = term;
    fit.current = addon;
    let disposed = false;
    let ready = false;
    let inputQueue = Promise.resolve();
    const offData = window.runtime?.EventsOn('terminal:' + session.id, (data: string) => {
      if (!disposed) term.write(Uint8Array.from(atob(data), (c) => c.charCodeAt(0)));
    });
    const offExit = window.runtime?.EventsOn('terminal-exit:' + session.id, (message: string) => {
      if (!disposed) {
        term.writeln('\r\n\x1b[90m' + message + '\x1b[0m');
        ready = false;
        term.options.disableStdin = true;
      }
    });
    const input = term.onData((data) => {
      if (!ready || !live.current) return;
      const bytes = new TextEncoder().encode(data);
      let binary = '';
      for (const b of bytes) binary += String.fromCharCode(b);
      inputQueue = inputQueue
        .then(() => {
          if (ready && live.current) return api.WriteTerminal(session.id, btoa(binary));
        })
        .catch((e) => {
          if (!disposed) setError(errorText(e));
        });
    });
    const resize = term.onResize(({ cols, rows }) => {
      if (ready) void api.ResizeTerminal(session.id, cols, rows).catch(() => {});
    });
    const observer = new ResizeObserver(() => {
      if (ref.current?.offsetWidth) addon.fit();
    });
    observer.observe(ref.current!);
    requestAnimationFrame(() => {
      if (disposed) return;
      addon.fit();
      if (!live.current) {
        term.options.disableStdin = true;
        return;
      }
      void api
        .StartSession(session.id, term.cols, term.rows)
        .then(() => {
          ready = true;
        })
        .catch((e) => {
          ready = false;
          if (!disposed) {
            setError(errorText(e));
            term.options.disableStdin = true;
          }
        });
    });
    return () => {
      disposed = true;
      observer.disconnect();
      input.dispose();
      resize.dispose();
      offData?.();
      offExit?.();
      term.dispose();
      terminal.current = null;
      fit.current = null;
    };
  }, [session.id]);
  useEffect(() => {
    if (active)
      requestAnimationFrame(() => {
        fit.current?.fit();
        terminal.current?.focus();
      });
  }, [active]);
  return (
    <div className="terminal-pane" style={{ display: active ? 'flex' : 'none' }}>
      <div className="terminal-caption">
        <span className={'status-dot ' + session.status} /> {session.name} ·{' '}
        {sessionStatus[session.status]}
        <span>中文提问或 /ai · /sidebar 展开 AI 与监控</span>
      </div>
      {!sessionLive(session) && (
        <div className="terminal-ended">
          <span>
            {session.status === 'failed'
              ? '会话异常结束，可查看终端信息后重试。'
              : '会话已结束，终端内容保留至关闭标签页。'}
          </span>
          <button className="button small" disabled={!canReconnect} onClick={onReconnect}>
            重新连接
          </button>
        </div>
      )}
      {error && <div className="terminal-error">{error}</div>}
      <div ref={ref} className="terminal-screen" />
    </div>
  );
}
