import { useEffect, useRef, useState } from 'react';
import { api, errorText } from '../api';
import { WidgetClient } from './client';
import { WidgetRenderer } from './Renderer';
import type { ViewNode, WidgetCandidate, WidgetData, WidgetMeta } from './protocol';
import './widgets.css';
const empty: WidgetData = {
  bindingId: '',
  sourceRevision: 0,
  sequence: 0,
  data: null,
  lastGood: null,
  updatedAt: '',
  lastGoodAt: '',
  error: '',
  issues: [],
  history: [],
  truncated: false,
  intervalMs: 2000,
};

export function WidgetPreviews({
  sessionId,
  candidates = [],
}: {
  sessionId: string;
  candidates?: WidgetCandidate[];
}) {
  const jobs = useRef(new Map<string, WidgetClient>());
  useEffect(() => {
    const ids = new Set(candidates.map((c) => c.id));
    for (const [id, client] of jobs.current)
      if (!ids.has(id)) {
        client.stop();
        jobs.current.delete(id);
      }
    for (const c of candidates) {
      if (jobs.current.has(c.id) || Date.parse(c.expiresAt) <= Date.now()) continue;
      const client = new WidgetClient();
      jobs.current.set(c.id, client);
      void (async () => {
        let failure = '';
        let stage = '初始化';
        try {
          const a = await api.GetWidgetArtifact(sessionId, c.hash);
          await client.init(a);
          if (c.sample) {
            stage = '真实 SSH 数据';
            await client.render(c.sample);
          }
          if (c.protocolVersion === 2) {
            stage = '真实 SSH 数据';
            if (!c.sample?.sequence || !c.sample.data) throw new Error('缺少真实 SSH 映射样本');
            await client.render(c.sample);
            stage = '未知指标状态';
            const unknown = Object.fromEntries(Object.entries(c.sample.data).map(([key, value]) => {
              const field = value as Record<string, unknown>;
              return [key, { ...field, ...(Object.hasOwn(field, 'rows') ? { rows: null } : { value: null }), quality: 'unknown', reason: '采集失败' }];
            }));
            await client.render({ ...c.sample, data: unknown, error: '采集失败' });
            stage = '过期数据状态';
            await client.render({ ...c.sample, data: null, lastGood: c.sample.data, error: '数据已过期' });
          }
          stage = '无采样数据';
          await client.render(empty);
          stage = '采集失败状态';
          await client.render({ ...empty, error: '采集失败' });
          for (const n of [0, 1, 1000000]) {
            if (c.protocolVersion === 2 || !c.data) break;
            stage = `契约样例 ${n}`;
            const fixture = Object.fromEntries(
              c.data.fields.map((f) => [
                f.name,
                f.type === 'number'
                  ? n
                  : f.type === 'boolean'
                    ? n !== 0
                    : f.type === 'table'
                      ? []
                      : '预检样例',
              ]),
            );
            await client.render({ ...empty, data: fixture });
          }
        } catch (e) {
          failure = `预检（${stage}）：${errorText(e)}`.slice(0, 300);
        } finally {
          client.stop();
        }
        if (!jobs.current.has(c.id)) return;
        try {
          await api.WorkbenchAction(sessionId, {
            kind: failure ? 'widget-failed' : 'widget-ready',
            panelId: c.panelId,
            candidateId: c.id,
            text: failure,
          });
        } catch {
          /* Expired candidates are cancelled by the owning App. */
        }
      })();
    }
  }, [sessionId, candidates]);
  useEffect(
    () => () => {
      for (const client of jobs.current.values()) client.stop();
      jobs.current.clear();
    },
    [sessionId],
  );
  return null;
}

export function WidgetHost({
  sessionId,
  panelId,
  meta,
  visible = true,
}: {
  sessionId: string;
  panelId: string;
  meta: WidgetMeta;
  visible?: boolean;
}) {
  const [tree, setTree] = useState<ViewNode[]>([]),
    [error, setError] = useState(''),
    [data, setData] = useState<WidgetData | null>(null),
    [retry, setRetry] = useState(0);
  const runtime = useRef<WidgetClient | null>(null);
  const busy = useRef(false);
  const generation = useRef(0);
  const waiting = useRef<WidgetData | null>(null);
  const refresh = useRef(() => {});
  const events = useRef({ start: 0, count: 0 });
  useEffect(() => {
    if (!visible) return;
    let disposed = false;
    const client = new WidgetClient();
    runtime.current = client;
    setError('');
    setTree([]);
    setData(null);
    waiting.current = null;
    busy.current = false;
    generation.current++;
    async function drain() {
      if (busy.current || disposed) return;
      busy.current = true;
      try {
        while (waiting.current && !disposed) {
          const next = waiting.current;
          waiting.current = null;
          const nodes = await client.render(next);
          if (!disposed) {
            generation.current++;
            setTree(nodes);
            setData(next);
            setError('');
          }
        }
      } catch (e) {
        if (!disposed) {
          setError(errorText(e));
          client.stop();
        }
      } finally {
        if (runtime.current === client) busy.current = false;
      }
    }
    let request = 0;
    async function load() {
      const id = ++request;
      try {
        const next = await api.GetWidgetData(sessionId, panelId, meta.bindingId);
        if (disposed || id !== request) return;
        waiting.current = next;
        await drain();
      } catch (e) {
        if (!disposed) setError(errorText(e));
      }
    }
    void (async () => {
      try {
        const a = await api.GetWidgetArtifact(sessionId, meta.hash);
        await client.init(a);
        if (disposed) return;
        refresh.current = () => void load();
        await load();
      } catch (e) {
        if (!disposed) setError(errorText(e));
      }
    })();
    return () => {
      disposed = true;
      runtime.current = null;
      refresh.current = () => {};
      waiting.current = null;
      client.stop();
    };
  }, [sessionId, panelId, meta.hash, meta.bindingId, retry, visible]);
  useEffect(() => refresh.current(), [meta.sequence]);
  const renderedGeneration = generation.current;
  async function event(id: string, payload: unknown) {
    if (busy.current || renderedGeneration !== generation.current || !runtime.current || error)
      return;
    const now = Date.now();
    if (now - events.current.start > 1000) events.current = { start: now, count: 0 };
    if (++events.current.count > 20) return;
    const client = runtime.current;
    busy.current = true;
    try {
      const nodes = await client.event(id, payload);
      if (runtime.current === client) {
        generation.current++;
        setTree(nodes);
      }
    } catch (e) {
      if (runtime.current === client) setError(errorText(e));
      client.stop();
    } finally {
      if (runtime.current === client) {
        busy.current = false;
        refresh.current();
      }
    }
  }
  if (!visible) return null;
  return (
    <div className="widget-host" aria-label="自定义实时组件">
      {error ? (
        <div className="widget-failure" role="status">
          <strong>组件展示已停止</strong>
          <span>{error}</span>
          <button type="button" className="button small" onClick={() => setRetry((v) => v + 1)}>
            重试组件
          </button>
        </div>
      ) : !tree.length ? (
        <p className="widget-empty">正在加载组件…</p>
      ) : (
        <WidgetRenderer tree={tree} onEvent={(id, payload) => void event(id, payload)} />
      )}
      {data?.error && (
        <p className="monitor-warning" role="status">
          数据无法使用：{data.error}
        </p>
      )}
      {!!data?.issues?.length && (
        <p className="monitor-warning">部分指标缺失：{data.issues.join('，')}</p>
      )}
      {data?.lastGoodAt && (
        <small className="widget-data-time">
          最近成功采集 {new Date(data.lastGoodAt).toLocaleTimeString()}
          {data.truncated ? ' · 历史显示实际保留范围' : ''}
        </small>
      )}
      {meta.saveError && (
        <p className="monitor-warning">
          已生效，未保存：{meta.saveError}
          <button
            className="button small"
            onClick={() =>
              void api.RetryWidgetSave(sessionId, panelId).catch((e) => setError(errorText(e)))
            }
          >
            重试保存
          </button>
        </p>
      )}
    </div>
  );
}
