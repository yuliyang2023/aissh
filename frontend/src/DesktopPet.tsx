import { useEffect, useRef, useState } from 'react';
import { BellOff, ExternalLink, X } from 'lucide-react';
import { AlertNotice, MonitoringState } from './monitoring-api';
import { errorText } from './api';

export function DesktopPet() {
  const [state, setState] = useState<MonitoringState | null>(null);
  const [bubble, setBubble] = useState<AlertNotice | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const latest = useRef(0);
  useEffect(() => {
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const next = await window.go!.desktop.PetBridge!.Snapshot();
        if (disposed) return;
        setState(next);
        setError('');
        const notice = next.notices.at(-1);
        if (notice && notice.sequence > latest.current) {
          latest.current = notice.sequence;
          if (Date.now() - Date.parse(notice.time) < 20000) setBubble(notice);
        }
      } catch (e) {
        if (!disposed) {
          setError(errorText(e));
          setBubble(null);
        }
      }
      if (!disposed) timer = setTimeout(poll, 1000);
    }
    void poll();
    return () => {
      disposed = true;
      clearTimeout(timer);
    };
  }, []);
  useEffect(() => {
    if (!bubble) return;
    const timer = setTimeout(() => setBubble(null), 20000);
    return () => clearTimeout(timer);
  }, [bubble]);
  async function action(kind: string, id = '') {
    if (busy) return;
    setBusy(true);
    try {
      await window.go!.desktop.PetBridge!.Action(kind, id);
      setError('');
      if (kind !== 'open') setBubble(null);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  const active = state?.incidents.filter((i) => i.status === 'firing') || [];
  const muted = !!state && Date.parse(state.snoozedUntil) > Date.now();
  const resting = muted || !state?.policy.enabled || !state.liveSessions;
  const mood = error
    ? 'warning'
    : active.some((i) => i.severity === 'critical')
      ? 'critical'
      : active.length
        ? 'warning'
        : resting
          ? 'resting'
          : 'watching';
  const visibleBubble =
    !muted &&
    state?.policy.enabled &&
    bubble &&
    (bubble.kind === 'recovered' ||
      active.some((i) => i.id === bubble.incident.id && !i.acknowledged))
      ? bubble
      : null;
  const label = error
    ? '连接中断'
    : !state
      ? '正在连接…'
      : !state.policy.enabled
        ? '提醒已关闭'
        : muted
          ? '安静陪伴中'
          : active.length
            ? `${active.length} 个问题需要关注`
            : state.liveSessions
              ? `正在守护 ${state.liveSessions} 个告警会话`
              : '到告警监控添加任务';
  return (
    <main className={'desktop-pet ' + mood}>
      <div className="pet-bubble-slot" aria-live="polite">
        {error ? (
          <div className="pet-bubble pet-error">{error}</div>
        ) : (
          visibleBubble && (
            <div
              className={'pet-bubble ' + (visibleBubble.kind === 'recovered' ? 'recovered' : '')}
            >
              <button
                className="pet-bubble-close"
                aria-label="收起气泡"
                onClick={() => setBubble(null)}
              >
                <X size={12} />
              </button>
              <button
                className="pet-bubble-open"
                disabled={busy}
                onClick={() => void action('open', visibleBubble.incident.sessionId)}
              >
                <small>
                  {visibleBubble.kind === 'recovered'
                    ? '已经恢复啦'
                    : visibleBubble.incident.severity === 'critical'
                      ? '有个问题需要你看看'
                      : '发现一个异常'}
                </small>
                <strong>{visibleBubble.incident.title}</strong>
                <span>{visibleBubble.incident.sessionName}</span>
                <p>{visibleBubble.incident.detail}</p>
              </button>
            </div>
          )
        )}
      </div>
      <div className="pet-character" title="拖动小猫移动窗口">
        <svg viewBox="0 0 180 126" role="img" aria-label="监控小猫">
          <ellipse cx="91" cy="117" rx="55" ry="6" fill="#000" opacity=".1" />
          <path
            className="pet-tail"
            d="M132 106 Q172 110 160 81"
            fill="none"
            stroke="#bbb2e3"
            strokeWidth="13"
            strokeLinecap="round"
          />
          <path
            d="M51 79 Q37 108 62 115 L125 115 Q142 101 125 79Z"
            fill="#dcd5f5"
            stroke="#71638e"
            strokeWidth="2.5"
          />
          <path
            d="M42 54 L43 13 Q44 5 51 10 L75 30 Q94 25 111 30 L135 10 Q142 5 142 14 L143 57 Q150 94 96 98 Q39 98 42 54Z"
            fill="#eee9fb"
            stroke="#71638e"
            strokeWidth="2.5"
            strokeLinejoin="round"
          />
          <path d="M51 20 L52 43 L68 33Z M132 20 L117 34 L134 44Z" fill="#d4a6bf" />
          <g
            className="pet-eyes"
            fill="none"
            stroke="#514968"
            strokeWidth="4.5"
            strokeLinecap="round"
          >
            <path d="M69 56 L69 63 M116 56 L116 63" />
          </g>
          <path d="M89 67 Q94 64 99 67 L94 72Z" fill="#ba839f" />
          <path
            d="M94 72 Q88 81 83 74 M94 72 Q99 81 105 74"
            fill="none"
            stroke="#71638e"
            strokeWidth="2.2"
            strokeLinecap="round"
          />
          <ellipse cx="60" cy="74" rx="9" ry="4" fill="#eecbdc" />
          <ellipse cx="128" cy="74" rx="9" ry="4" fill="#eecbdc" />
          <path d="M68 96 L68 111 M117 96 L117 111" stroke="#b2a5d3" strokeWidth="2" />
          <circle
            className="pet-badge"
            cx="94"
            cy="100"
            r="8"
            fill="#61a78b"
            stroke="#fff"
            strokeWidth="2"
          />
        </svg>
      </div>
      <button className="pet-status" disabled={busy} onClick={() => void action('open')}>
        <span />
        {label}
      </button>
      <div className="pet-tools">
        <button
          disabled={busy}
          aria-label="打开告警监控"
          title="打开告警监控"
          onClick={() => void action('open')}
        >
          <ExternalLink size={14} />
        </button>
        <button
          disabled={busy}
          aria-label={muted ? '恢复提醒' : '静音 15 分钟'}
          title={muted ? '恢复提醒' : '静音 15 分钟'}
          onClick={() => void action(muted ? 'resume' : 'snooze')}
        >
          <BellOff size={14} />
        </button>
        <button
          disabled={busy}
          aria-label="隐藏桌宠"
          title="隐藏桌宠，后台监控继续"
          onClick={() => void action('hide')}
        >
          <X size={14} />
        </button>
      </div>
    </main>
  );
}
