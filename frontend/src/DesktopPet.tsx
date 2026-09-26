import { useEffect, useRef, useState } from 'react';
import { PixelCat, PixelIcon, PetMood } from './PixelCat';
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
  const mood: PetMood = error
    ? 'warning'
    : muted || !state?.policy.enabled
      ? 'resting'
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
                <PixelIcon kind="close" />
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
      <div className="pet-character" title="拖动小猫移动位置">
        <PixelCat mood={mood} />
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
          <PixelIcon kind="open" />
        </button>
        <button
          disabled={busy}
          aria-label={muted ? '恢复提醒' : '静音 15 分钟'}
          title={muted ? '恢复提醒' : '静音 15 分钟'}
          onClick={() => void action(muted ? 'resume' : 'snooze')}
        >
          <PixelIcon kind={muted ? 'muted' : 'sound'} />
        </button>
        <button
          disabled={busy}
          aria-label="隐藏桌宠"
          title="隐藏桌宠，后台监控继续"
          onClick={() => void action('hide')}
        >
          <PixelIcon kind="close" />
        </button>
      </div>
    </main>
  );
}
