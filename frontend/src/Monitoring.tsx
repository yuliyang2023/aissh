import { useEffect, useState } from 'react';
import { BellOff, Check, CircleAlert, Settings2, Sparkles } from 'lucide-react';
import { api, errorText, Session, sessionLive } from './api';
import { ErrorBox, Field, Modal } from './components';
import {
  AlertPolicy,
  MonitoringState,
  PetStyle,
  petStyles,
  alertMonitorPrompt,
} from './monitoring-api';
import { MonitorInventory } from './MonitorInventory';
import { PixelCat } from './PixelCat';

export type MonitoringView = 'monitors' | 'incidents';

export function useMonitoringState() {
  const [state, setState] = useState<MonitoringState | null>(null);
  const [error, setError] = useState('');
  useEffect(() => {
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    async function refresh() {
      try {
        const next = await api.GetMonitoring();
        if (!disposed) setState(next);
      } catch (e) {
        if (!disposed) setError(errorText(e));
      }
      if (!disposed) timer = setTimeout(refresh, 1000);
    }
    void refresh();
    return () => {
      disposed = true;
      clearTimeout(timer);
    };
  }, []);
  return { state, setState, error, setError };
}

export function Monitoring({
  onOpenSession,
  sessions,
  activeSession,
  inventoryRequest,
  monitoring,
  pageVisible,
  onOpenPage,
}: {
  onOpenSession: (id: string) => void;
  sessions: Session[];
  activeSession: string;
  inventoryRequest: { sequence: number; sessionId: string; view: MonitoringView };
  monitoring: ReturnType<typeof useMonitoringState>;
  pageVisible: boolean;
  onOpenPage: () => void;
}) {
  const { state, setState, error, setError } = monitoring;
  const [opened, setOpened] = useState(false);
  const [view, setView] = useState<'monitors' | 'incidents' | 'policy'>('monitors');
  const [editing, setEditing] = useState<AlertPolicy | null>(null);
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const [aiOpen, setAIOpen] = useState(false);
  const [aiPrompt, setAIPrompt] = useState('');
  const [aiSession, setAISession] = useState('');
  const [scopeSession, setScopeSession] = useState('');
  const live = sessions.filter(sessionLive);
  const targets = scopeSession ? live.filter((s) => s.id === scopeSession) : live;
  const target =
    targets.find((s) => s.id === aiSession) ||
    targets.find((s) => s.id === activeSession) ||
    targets[0];
  const scopeName = sessions.find((s) => s.id === scopeSession)?.name || '已结束的会话';
  const scopedState =
    state && scopeSession
      ? {
          ...state,
          monitors: (state.monitors || []).filter((m) => m.sessionId === scopeSession),
          incidents: state.incidents.filter((i) => i.sessionId === scopeSession),
        }
      : state;
  useEffect(() => {
    if (inventoryRequest.sequence) {
      setScopeSession(inventoryRequest.sessionId);
      setAISession(inventoryRequest.sessionId);
      setAIOpen(false);
      setAIPrompt('');
      setView(inventoryRequest.view);
      setEditing(null);
      setSaved(false);
      setOpened(!!inventoryRequest.sessionId);
    }
  }, [inventoryRequest]);
  useEffect(
    () =>
      window.runtime?.EventsOn('monitor-session', (id: string) => {
        onOpenSession(id);
        setOpened(false);
      }),
    [onOpenSession],
  );
  async function act(action: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    setError('');
    setSaved(false);
    try {
      await action();
      setState(await api.GetMonitoring());
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  const active = scopedState?.incidents.filter((i) => i.status === 'firing') || [];
  const critical = active.filter((i) => i.severity === 'critical').length;
  const unacknowledged = active.filter((i) => !i.acknowledged).length;
  const muted = !!state && Date.parse(state.snoozedUntil) > Date.now();
  const content = (
    <div className="monitoring-body">
      <ErrorBox error={error || state?.error || ''} />
      {!state || !scopedState ? (
        <p>正在读取监控状态…</p>
      ) : (
        <>
          <div className={'monitoring-overview ' + (active.length ? 'has-alerts' : '')}>
            {active.length > 0 ? <CircleAlert size={22} /> : <Check size={22} />}
            <div>
              <strong>
                {active.length
                  ? `${active.length} 个问题需要关注`
                  : scopedState.monitors.length
                    ? '暂无已触发的告警'
                    : '尚未配置告警任务'}
              </strong>
              <small>
                {active.length
                  ? `${critical} 个严重 · ${active.length - critical} 个注意 · ${unacknowledged} 个未确认`
                  : scopeSession
                    ? '查看下方任务的触发条件与采集状态'
                    : `${state.liveSessions} 个会话有告警任务`}
              </small>
            </div>
            {scopeSession ? (
              active.length > 0 &&
              view !== 'incidents' && (
                <button
                  className="button small"
                  disabled={busy}
                  onClick={() => {
                    setView('incidents');
                    setAIOpen(false);
                  }}
                >
                  查看异常
                </button>
              )
            ) : (
              <button
                className="button small"
                disabled={busy}
                onClick={() => void act(() => api.SnoozeAlerts(muted ? 0 : 15))}
              >
                <BellOff size={14} />
                {muted ? '恢复提醒' : '静音 15 分钟'}
              </button>
            )}
          </div>
          {(!state.policy.enabled || muted) && (
            <p className="monitoring-reminder-status" role="status">
              <BellOff size={14} />
              {!state.policy.enabled
                ? '全局异常判定与提醒已关闭，任务仍会采集数据'
                : `提醒已暂时静音，至 ${new Date(state.snoozedUntil).toLocaleTimeString()}；异常仍会记录`}
            </p>
          )}
          {!scopeSession && (
            <div className="monitoring-pet-setting">
              <span className={`pet-preview pet-style-${state.policy.petStyle || 'classic'}`}>
                <PixelCat mood="watching" style={state.policy.petStyle} />
              </span>
              <div>
                <strong>让监控小猫陪你值班</strong>
                <small>独立悬浮窗口 · 拖动小猫移动 · 点击气泡查看会话</small>
                <label className="pet-style-picker">
                  桌宠样式
                  <select
                    value={state.policy.petStyle || 'classic'}
                    disabled={busy}
                    onChange={(e) => void act(() => api.SetPetStyle(e.target.value as PetStyle))}
                  >
                    {petStyles.map((style) => (
                      <option key={style.id} value={style.id}>
                        {style.label}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              <button
                className="button small"
                disabled={busy}
                onClick={() => void act(() => api.SetPetEnabled(!state.petRunning))}
              >
                {state.petRunning ? '关闭桌宠' : '开启桌宠'}
              </button>
            </div>
          )}
          <div className="monitoring-tabs" role="group" aria-label="提醒视图">
            <button
              className={'button small ' + (view === 'monitors' ? 'primary' : '')}
              aria-pressed={view === 'monitors'}
              disabled={busy}
              onClick={() => {
                setView('monitors');
                setEditing(null);
              }}
            >
              告警任务（{scopedState.monitors?.length || 0}）
            </button>
            <button
              className={'button small ' + (view === 'incidents' ? 'primary' : '')}
              aria-pressed={view === 'incidents'}
              disabled={busy}
              onClick={() => {
                setEditing(null);
                setView('incidents');
                setAIOpen(false);
              }}
            >
              问题记录
              {active.length > 0 && <span className="monitoring-tab-count">{active.length}</span>}
            </button>
            {!scopeSession && (
              <button
                className={'button small ' + (view === 'policy' ? 'primary' : '')}
                aria-pressed={view === 'policy'}
                disabled={busy}
                onClick={() => {
                  setEditing(structuredClone(state.policy));
                  setView('policy');
                  setSaved(false);
                  setAIOpen(false);
                }}
              >
                <Settings2 size={14} />
                全局提醒策略
              </button>
            )}
            {view === 'monitors' && (
              <button
                className="button small monitor-add-button"
                disabled={busy || !target}
                aria-expanded={aiOpen}
                onClick={() => setAIOpen(!aiOpen)}
              >
                <Sparkles size={14} />
                AI 添加告警任务
              </button>
            )}
            {saved && <span role="status">策略已保存</span>}
          </div>
          {aiOpen && (
            <form
              className="monitor-ai-form"
              onSubmit={(e) => {
                e.preventDefault();
                if (!target || !aiPrompt.trim()) return;
                void act(async () => {
                  await api.WorkbenchAction(target.id, {
                    kind: 'prompt',
                    text: alertMonitorPrompt(aiPrompt),
                  });
                  setAIOpen(false);
                  setAIPrompt('');
                  await api.OpenMonitorSession(target.id);
                });
              }}
            >
              <Field label="监控哪个会话">
                <select
                  value={target?.id || ''}
                  disabled={!!scopeSession || busy}
                  onChange={(e) => setAISession(e.target.value)}
                >
                  {!target && <option value="">当前会话未连接</option>}
                  {targets.map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="希望 AI 在什么情况下提醒">
                <textarea
                  rows={3}
                  required
                  maxLength={4000}
                  placeholder="例如：监控 nginx 的运行状态，停止超过 10 秒提醒；也监控日志中的 timeout"
                  value={aiPrompt}
                  onChange={(e) => setAIPrompt(e.target.value)}
                />
              </Field>
              <div>
                <small>将使用所选会话的 AI，在对话中创建独立告警任务和提醒规则。</small>
                <button
                  className="button primary small"
                  disabled={busy || !target || !aiPrompt.trim()}
                >
                  让 AI 创建
                </button>
              </div>
            </form>
          )}
          {view === 'monitors' && (
            <MonitorInventory
              key={scopeSession}
              scoped={!!scopeSession}
              state={scopedState}
              busy={busy}
              onOpenSession={(id) => void act(() => api.OpenMonitorSession(id))}
              onAction={(id, panelId, kind) =>
                void act(() => api.WorkbenchAction(id, { kind, panelId }))
              }
            />
          )}
          {!scopeSession && editing ? (
            <form
              className="alert-policy"
              onSubmit={(e) => {
                e.preventDefault();
                void act(async () => {
                  await api.SaveAlertPolicy(editing);
                  setEditing(null);
                  setView('incidents');
                  setSaved(true);
                });
              }}
            >
              <label className="alert-check">
                <input
                  type="checkbox"
                  checked={editing.enabled}
                  onChange={(e) => setEditing({ ...editing, enabled: e.target.checked })}
                />
                启用异常判定与提醒
              </label>
              <div className="alert-policy-timing">
                {(
                  [
                    ['confirmSeconds', '采集/断线异常持续（秒）', 0, 3600],
                    ['recoverySeconds', '采集/断线恢复持续（秒）', 0, 3600],
                    ['repeatMinutes', '重复提醒间隔（分钟）', 1, 1440],
                    ['staleSeconds', '数据过期时间（秒）', 15, 3600],
                  ] as const
                ).map(([key, label, min, max]) => (
                  <Field key={key} label={label}>
                    <input
                      type="number"
                      required
                      min={min}
                      max={max}
                      value={editing[key]}
                      onChange={(e) => setEditing({ ...editing, [key]: e.target.valueAsNumber })}
                    />
                  </Field>
                ))}
              </div>
              <div className="alert-rules">
                {editing.rules.map((rule, index) => {
                  const update = (changes: Partial<typeof rule>) =>
                    setEditing({
                      ...editing,
                      rules: editing.rules.map((r, i) => (i === index ? { ...r, ...changes } : r)),
                    });
                  return (
                    <div key={rule.id} className="alert-rule">
                      <label className="alert-check">
                        <input
                          type="checkbox"
                          checked={rule.enabled}
                          onChange={(e) => update({ enabled: e.target.checked })}
                        />
                        {rule.name}
                      </label>
                      <span />
                      <select
                        aria-label={`${rule.name}级别`}
                        value={rule.severity}
                        onChange={(e) =>
                          update({ severity: e.target.value as 'warning' | 'critical' })
                        }
                      >
                        <option value="warning">注意</option>
                        <option value="critical">严重</option>
                      </select>
                    </div>
                  );
                })}
              </div>
              <label className="alert-check">
                <input
                  type="checkbox"
                  checked={editing.notifyRecovery}
                  onChange={(e) => setEditing({ ...editing, notifyRecovery: e.target.checked })}
                />
                问题恢复时提醒
              </label>
              <p className="monitoring-note">
                此策略适用于所有会话的告警任务，管理采集失败、数据过期和断线提醒。业务阈值与持续时间由每个告警任务单独配置；实时面板不参与判定。
              </p>
              <button className="button primary" disabled={busy}>
                保存提醒策略
              </button>
            </form>
          ) : view === 'incidents' ? (
            <div className="alert-history" aria-label="问题记录">
              {scopedState.incidents.length === 0 ? (
                <div className="alert-empty">
                  <Check size={24} />
                  <strong>暂时没有问题记录</strong>
                  <p>在「告警任务」中添加需要提醒的项目，触发和恢复会记录在这里。</p>
                </div>
              ) : (
                [...scopedState.incidents]
                  .sort(
                    (a, b) =>
                      Number(b.status === 'firing') - Number(a.status === 'firing') ||
                      (a.status === 'firing' && b.status === 'firing'
                        ? Number(a.acknowledged) - Number(b.acknowledged) ||
                          Number(b.severity === 'critical') - Number(a.severity === 'critical')
                        : 0) ||
                      Date.parse(b.updatedAt) - Date.parse(a.updatedAt),
                  )
                  .map((item) => (
                    <article
                      className={'alert-incident ' + item.status + ' ' + item.severity}
                      key={item.id}
                    >
                      <div className="alert-incident-title">
                        <strong>{item.title}</strong>
                        <span>
                          {item.status === 'recovered'
                            ? '已恢复'
                            : item.status === 'stopped'
                              ? '已停止监控'
                              : item.acknowledged
                                ? '已知晓 · 仍异常'
                                : item.severity === 'critical'
                                  ? '严重'
                                  : '注意'}
                        </span>
                      </div>
                      <small>
                        {item.sessionName} · {new Date(item.openedAt).toLocaleString()}
                      </small>
                      <p>{item.detail}</p>
                      <div className="alert-incident-actions">
                        <button
                          className="button small"
                          disabled={busy}
                          onClick={() => void act(() => api.OpenMonitorSession(item.sessionId))}
                        >
                          查看会话
                        </button>
                        {item.status === 'firing' && !item.acknowledged && (
                          <button
                            className="button small"
                            disabled={busy}
                            onClick={() => void act(() => api.AcknowledgeAlert(item.id))}
                          >
                            我知道了
                          </button>
                        )}
                      </div>
                    </article>
                  ))
              )}
            </div>
          ) : null}
          <div className="monitoring-footer">
            <details className="monitoring-help">
              <summary>监控与提醒说明</summary>
              <p className="monitoring-note">
                客户端运行且 SSH 会话保持连接时监控生效，执行命令时采集可能延后。
                休眠或退出客户端会停止监控，问题记录仅保留在本次运行中。
                “我知道了”会停止该问题的重复提醒，不代表问题已恢复。
                采集失败、数据过期和断线沿用全局提醒策略。
              </p>
            </details>
            {scopeSession && (
              <button className="button small" disabled={busy} onClick={onOpenPage}>
                全部连接与全局设置
              </button>
            )}
          </div>
        </>
      )}
    </div>
  );
  if (pageVisible) {
    return (
      <section className="page-content monitoring-page" aria-label="告警监控">
        {content}
      </section>
    );
  }
  return opened ? (
    <Modal
      title="连接告警"
      subtitle={`${scopeName} · 仅显示此连接的告警任务与问题`}
      wide
      className="monitoring-modal"
      onClose={() => {
        if (!busy) {
          setOpened(false);
          setEditing(null);
        }
      }}
    >
      {content}
    </Modal>
  ) : null;
}
