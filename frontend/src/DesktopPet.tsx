import { useEffect, useReducer, useState } from 'react';
import { PixelCat, PixelIcon, PetMood } from './PixelCat';
import { PetState, petStyles } from './monitoring-api';
import { emptyPetInbox, petInboxReducer, petSessionLabel, PetNotice } from './pet-notices';
import { errorText } from './api';

export function DesktopPet() {
  const [state, setState] = useState<PetState | null>(null);
  const [inbox, dispatch] = useReducer(petInboxReducer, emptyPetInbox);
  const bubble = inbox.items.find((item) => item.id === inbox.selected);
  const [interacting, setInteracting] = useState(false);
  const unreadSessions = new Set(inbox.items.map((item) => item.sessionId)).size;
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const next = await window.go!.desktop.PetBridge!.Snapshot();
        if (disposed) return;
        setState(next);
        setError('');
        dispatch({ type: 'snapshot', state: next, now: Date.now() });
      } catch (e) {
        if (!disposed) {
          setError(errorText(e));
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
    if (!inbox.open || interacting || inbox.view === 'list') return;
    const timer = setTimeout(() => dispatch({ type: 'hide' }), 20000);
    return () => clearTimeout(timer);
  }, [inbox.open, inbox.view, bubble?.id, interacting]);
  async function action(kind: string, id = '', notice?: PetNotice) {
    if (busy) return;
    setBusy(true);
    try {
      await window.go!.desktop.PetBridge!.Action(kind, id);
      if (kind === 'style' || kind === 'snooze' || kind === 'resume') {
        const next = await window.go!.desktop.PetBridge!.Snapshot();
        setState(next);
        dispatch({ type: 'snapshot', state: next, now: Date.now() });
      }
      setError('');
      if (kind === 'open' && notice) dispatch({ type: 'read', id: notice.id });
      if (kind === 'hide') dispatch({ type: 'hide' });
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  function moveNotice(delta: number) {
    const index = inbox.items.findIndex((item) => item.id === inbox.selected);
    const next = inbox.items[(index + delta + inbox.items.length) % inbox.items.length];
    if (next) dispatch({ type: 'select', id: next.id });
  }
  const active = state?.incidents.filter((i) => i.status === 'firing') || [];
  const styleIndex = Math.max(
    0,
    petStyles.findIndex((style) => style.id === state?.policy.petStyle),
  );
  const style = petStyles[styleIndex].id;
  const nextStyle = petStyles[(styleIndex + 1) % petStyles.length];
  const changeStyleLabel = `切换为${nextStyle.label}`;
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
          : bubble?.taskKind && !muted && state?.policy.enabled
            ? bubble.taskKind === 'completed'
              ? 'watching'
              : 'warning'
            : resting
              ? 'resting'
              : 'watching';
  const visibleBubble =
    inbox.open &&
    !muted &&
    state?.policy.enabled &&
    bubble &&
    (!bubble.alert ||
      bubble.alert.kind === 'recovered' ||
      active.some((i) => i.id === bubble.alert!.incident.id && !i.acknowledged))
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
          : inbox.items.length
            ? `${unreadSessions} 个会话 · ${inbox.items.length} 条未读`
            : active.length
              ? `${active.length} 个问题需要关注`
              : visibleBubble?.taskKind
                ? visibleBubble.title
                : state.liveSessions
                  ? `正在守护 ${state.liveSessions} 个告警会话`
                  : '任务完成后会提醒你';
  return (
    <main
      className={`desktop-pet pet-style-${style} ${mood} ${inbox.open && inbox.view === 'list' ? 'pet-list-open' : ''}`}
    >
      <div
        className="pet-bubble-slot"
        aria-live="polite"
        onMouseEnter={() => setInteracting(true)}
        onMouseLeave={() => setInteracting(false)}
        onFocusCapture={() => setInteracting(true)}
        onBlurCapture={(event) => {
          if (!event.currentTarget.contains(event.relatedTarget)) setInteracting(false);
        }}
        onKeyDown={(event) => {
          if (event.key === 'Escape') {
            dispatch({ type: 'hide' });
            event.stopPropagation();
          }
        }}
      >
        {error ? (
          <div className="pet-bubble pet-error">{error}</div>
        ) : visibleBubble && inbox.view === 'list' ? (
          <section className="pet-bubble pet-inbox" aria-label="未读提醒列表">
            <div className="pet-inbox-header">
              <strong>未读提醒 · {inbox.items.length}</strong>
              <button
                className="pet-bubble-close"
                aria-label="收起提醒列表"
                onClick={() => dispatch({ type: 'hide' })}
              >
                <PixelIcon kind="close" />
              </button>
            </div>
            <div className="pet-inbox-items">
              {inbox.items.map((item) => (
                <button
                  key={item.id}
                  className={`pet-inbox-item ${item.positive ? 'positive' : 'attention'}`}
                  disabled={busy}
                  onClick={() => void action('open', item.sessionId, item)}
                  title={`${petSessionLabel(item, inbox.items)} · ${item.title}`}
                >
                  <span className="pet-inbox-dot" />
                  <span>
                    <strong>{item.title}</strong>
                    <small>{petSessionLabel(item, inbox.items)}</small>
                  </span>
                  <PixelIcon kind="open" />
                </button>
              ))}
            </div>
            <div className="pet-inbox-footer">
              <span>点击提醒查看对应会话</span>
              <button disabled={busy} onClick={() => dispatch({ type: 'clear' })}>
                全部已读
              </button>
            </div>
          </section>
        ) : visibleBubble ? (
          <div className={'pet-bubble ' + (visibleBubble.positive ? 'recovered' : '')}>
            <button
              className="pet-bubble-close"
              aria-label="收起气泡"
              onClick={() => dispatch({ type: 'hide' })}
            >
              <PixelIcon kind="close" />
            </button>
            <button
              className="pet-bubble-open"
              disabled={busy}
              onClick={() => void action('open', visibleBubble.sessionId, visibleBubble)}
            >
              <small>{visibleBubble.heading}</small>
              <strong>{visibleBubble.title}</strong>
              <span title={petSessionLabel(visibleBubble, inbox.items)}>
                {petSessionLabel(visibleBubble, inbox.items)}
              </span>
              <p>{visibleBubble.detail}</p>
            </button>
            <div className="pet-notice-controls">
              <button
                aria-label="上一条提醒"
                disabled={inbox.items.length < 2 || busy}
                onClick={() => moveNotice(-1)}
              >
                ‹
              </button>
              <span aria-label="提醒位置">
                {inbox.items.findIndex((item) => item.id === visibleBubble.id) + 1} /{' '}
                {inbox.items.length}
              </span>
              <button
                aria-label="下一条提醒"
                disabled={inbox.items.length < 2 || busy}
                onClick={() => moveNotice(1)}
              >
                ›
              </button>
              <button
                className="pet-notice-read"
                disabled={busy}
                onClick={() => dispatch({ type: 'read', id: visibleBubble.id })}
              >
                已读
              </button>
              <button onClick={() => dispatch({ type: 'list' })}>全部提醒</button>
            </div>
          </div>
        ) : null}
      </div>
      <div className="pet-character" title="拖动小猫移动位置">
        <PixelCat mood={mood} style={style} />
      </div>
      <button
        className="pet-status"
        disabled={busy}
        aria-expanded={inbox.items.length ? inbox.open : undefined}
        onClick={() =>
          inbox.items.length
            ? dispatch({ type: inbox.open && inbox.view === 'list' ? 'hide' : 'list' })
            : void action('open')
        }
      >
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
          disabled={busy || !state}
          aria-label={changeStyleLabel}
          title={changeStyleLabel}
          onClick={() => void action('style', nextStyle.id)}
        >
          <PixelIcon kind="style" />
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
