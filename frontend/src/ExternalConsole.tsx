import { useEffect, useState } from 'react';
import { api, Catalog, errorText, Resource } from './api';
import { ErrorBox, Field } from './components';
import {
  ExternalDefinition,
  ExternalGrant,
  ExternalPreview,
  ExternalState,
  ExternalView,
  permissionLabels,
  riskLabels,
  RiskPolicy,
} from './externals-api';
import './externals.css';
const json = (value: unknown) => JSON.stringify(value, null, 2);
const toggle = (values: string[], key: string) =>
  values.includes(key) ? values.filter((v) => v !== key) : [...values, key];
const emptySchema = { type: 'object', properties: {}, additionalProperties: false };
const effectLabels = {
  destructive: '删除或清空',
  serviceImpact: '服务启停或中断',
  permissionChange: '远端权限变更',
  sensitive: '敏感数据',
} as const;
const outcomeLabels: Record<string, string> = {
  started: '已发起，等待结果',
  succeeded: '成功',
  failed: '失败',
  outcome_unknown: '结果不确定，先核对状态',
  denied: '被规则拒绝',
};

export function ExternalConsole({
  resource,
  data,
  onExplore,
}: {
  resource: Resource;
  data: Catalog;
  onExplore: () => Promise<void>;
}) {
  const [view, setView] = useState<ExternalView>();
  const [tab, setTab] = useState('operations');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [editing, setEditing] = useState<ExternalDefinition>();
  const [copyFrom, setCopyFrom] = useState('');
  const [reason, setReason] = useState('');
  const [filter, setFilter] = useState('all');
  const refresh = async () => setView(await api.GetExternalState(resource.id));
  useEffect(() => {
    void refresh().catch((e) => setError(errorText(e)));
  }, [resource.id]);
  useEffect(() => {
    if (view?.job.status !== 'running') return;
    const timer = setInterval(() => void refresh().catch((e) => setError(errorText(e))), 1200);
    return () => clearInterval(timer);
  }, [view?.job.status, resource.id]);
  async function run(action: () => Promise<unknown>) {
    setBusy(true);
    setError('');
    try {
      await action();
      await refresh();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  function setState(state: ExternalState) {
    setView((v) => (v ? { ...v, state } : v));
  }
  const state = view?.state;
  const report = view?.discovery || resource.discovery;
  const allOps = [...(state?.operations || []), ...(state?.candidates || [])];
  const siblings = data.resources.filter(
    (r) => r.id !== resource.id && r.kind === resource.kind && r.systemId === resource.systemId,
  );
  return (
    <div className="external-console">
      <div className="external-toolbar">
        <div>
          <strong>{resource.systemName || resource.name}</strong>
          <small>
            {resource.environment || 'production'} ·{' '}
            {resource.route === 'ssh' ? 'SSH 隧道' : '本机直连'}
            {state ? ' · 目录版本 ' + state.revision : ''}
          </small>
        </div>
        <div>
          <button className="button small" disabled={busy} onClick={() => void run(refresh)}>
            刷新
          </button>{' '}
          <button
            className="button small"
            disabled={busy || view?.job.status === 'running'}
            onClick={() => void run(onExplore)}
          >
            {view?.job.status === 'running' ? '正在探索…' : '只读探索'}
          </button>
          {view?.job.status === 'running' && (
            <button
              className="button small"
              onClick={() => void run(() => api.CancelResourceDiscovery(resource.id))}
            >
              取消探索
            </button>
          )}
        </div>
      </div>
      <p className="external-summary">
        {report?.summary || '读取接口文档或服务元数据后，候选接口会出现在这里。'}
        {view?.job.error && ' ' + view.job.error}
      </p>
      <nav className="external-tabs" aria-label="外联系统管理">
        {Object.entries({
          operations: '接口目录',
          permissions: '权限管理',
          risk: '风险规则',
          invoke: '预览与调用',
          audit: '审计记录',
        }).map(([key, label]) => (
          <button
            className={'button ' + (tab === key ? 'primary' : '')}
            key={key}
            onClick={() => {
              setTab(key);
              setError('');
            }}
          >
            {label}
          </button>
        ))}
      </nav>
      <ErrorBox error={error} />
      {!state ? (
        <p>正在加载接口与规则…</p>
      ) : (
        <fieldset disabled={busy} className="external-content">
          {tab === 'operations' && (
            <>
              <div className="external-toolbar">
                <span>
                  候选 {state.candidates?.length || 0} · 已登记 {state.operations.length}
                </span>
                <button
                  className="button small"
                  onClick={() =>
                    setEditing({
                      id: '',
                      revision: 0,
                      name: '',
                      kind: resource.kind,
                      description: '',
                      inputSchema: emptySchema,
                      request:
                        resource.kind === 'http'
                          ? { method: 'GET', path: '/', parameters: [] }
                          : resource.kind === 'redis'
                            ? { redisCommand: 'GET', redisParameters: ['key'], parameters: [] }
                            : { query: 'SELECT 1', sqlParameters: [], parameters: [] },
                      assessment: {
                        effects: {
                          access: 'unknown',
                          destructive: false,
                          serviceImpact: false,
                          permissionChange: false,
                          sensitive: false,
                        },
                        risk: 'unknown',
                        assurance: 'inferred',
                        evidence: '手工定义',
                      },
                      lifecycle: 'draft',
                      source: 'manual',
                      updatedAt: '',
                    })
                  }
                >
                  新增接口
                </button>
              </div>
              <p className="external-hint">
                确认接口定义与授予调用权限分开管理。修改接口会产生新版本，原授权不会自动继承。
              </p>
              {editing ? (
                <OperationEditor
                  key={editing.id || 'new'}
                  value={editing}
                  onCancel={() => setEditing(undefined)}
                  onSave={async (op, why) => {
                    await run(async () => {
                      setState(
                        await api.SaveExternalOperation(resource.id, state.revision, op, why),
                      );
                      setEditing(undefined);
                    });
                  }}
                />
              ) : (
                <>
                  <div className="external-toolbar">
                    <label>
                      筛选{' '}
                      <select
                        aria-label="接口筛选"
                        value={filter}
                        onChange={(e) => setFilter(e.target.value)}
                      >
                        <option value="all">全部</option>
                        <option value="draft">待确认</option>
                        <option value="read">读取</option>
                        <option value="write">写入</option>
                        <option value="dangerous">有危险效果</option>
                      </select>
                    </label>
                  </div>
                  <div className="external-operation-list">
                    {allOps
                      .filter(
                        (op) =>
                          filter === 'all' ||
                          filter === op.lifecycle ||
                          filter === op.assessment.effects.access ||
                          (filter === 'dangerous' &&
                            Object.keys(effectLabels).some(
                              (key) => op.assessment.effects[key as keyof typeof effectLabels],
                            )),
                      )
                      .map((op, index) => (
                        <article className="external-operation" key={op.id + index}>
                          <div className="external-toolbar">
                            <strong>
                              {op.name} <small>v{op.revision}</small>
                            </strong>
                            <button className="button small" onClick={() => setEditing(op)}>
                              {op.lifecycle === 'draft' ? '审查并编辑' : '编辑定义'}
                            </button>
                          </div>
                          <div className="external-badges">
                            <span>
                              {op.lifecycle === 'active'
                                ? '已启用'
                                : op.lifecycle === 'draft'
                                  ? '待确认'
                                  : '已停用'}
                            </span>
                            <span>
                              {op.assessment.effects.access === 'read'
                                ? '只读'
                                : op.assessment.effects.access === 'write'
                                  ? '写入'
                                  : '效果待确认'}
                            </span>
                            <span>风险：{riskLabels[op.assessment.risk]}</span>
                            {Object.entries(effectLabels)
                              .filter(
                                ([key]) => op.assessment.effects[key as keyof typeof effectLabels],
                              )
                              .map(([key, label]) => (
                                <span key={key}>{label}</span>
                              ))}
                          </div>
                          <code>
                            {op.request.method
                              ? `${op.request.method} ${op.request.path}`
                              : op.request.query || op.request.redisCommand}
                          </code>
                          {op.blocked && <p className="external-blocked">{op.blocked}</p>}
                          <small>{op.assessment.evidence}</small>
                        </article>
                      ))}
                  </div>
                  {!allOps.length && <p>尚无接口。可开始探索，或手动添加经过核对的接口定义。</p>}
                  {!!siblings.length && (
                    <details>
                      <summary>复用同一系统其他环境的接口</summary>
                      <Field label="来源连接">
                        <select value={copyFrom} onChange={(e) => setCopyFrom(e.target.value)}>
                          <option value="">请选择</option>
                          {siblings.map((r) => (
                            <option key={r.id} value={r.id}>
                              {r.name} · {r.environment}
                            </option>
                          ))}
                        </select>
                      </Field>
                      <Field label="复用原因">
                        <input value={reason} onChange={(e) => setReason(e.target.value)} />
                      </Field>
                      <button
                        className="button"
                        disabled={!copyFrom || !reason.trim()}
                        onClick={() =>
                          void run(() =>
                            api.CopyExternalDefinitions(
                              copyFrom,
                              resource.id,
                              state.revision,
                              reason,
                            ),
                          )
                        }
                      >
                        复制为候选接口
                      </button>
                      <p>参数契约可复用；当前环境仍须审查并授权。</p>
                    </details>
                  )}
                  {report?.notes?.length ? (
                    <details>
                      <summary>探索范围与记录</summary>
                      <ul>
                        {report.notes.map((n, i) => (
                          <li key={i}>{n}</li>
                        ))}
                      </ul>
                    </details>
                  ) : null}
                </>
              )}
            </>
          )}
          {tab === 'permissions' && (
            <PermissionsEditor
              key={state.revision}
              state={state}
              data={data}
              save={(policy, grants, why) =>
                run(async () =>
                  setState(
                    await api.SaveExternalPermissions(
                      resource.id,
                      state.revision,
                      policy,
                      grants,
                      why,
                    ),
                  ),
                )
              }
            />
          )}
          {tab === 'risk' && (
            <RiskEditor
              key={state.revision}
              value={state.risk}
              save={(risk, why) =>
                run(async () =>
                  setState(await api.SaveExternalRisk(resource.id, state.revision, risk, why)),
                )
              }
            />
          )}
          {tab === 'invoke' && (
            <InvocationEditor resource={resource} state={state} data={data} run={run} />
          )}
          {tab === 'audit' && (
            <>
              <h3>调用记录</h3>
              <p className="external-hint">
                仅记录请求摘要、规则版本和结果，不保存凭据或业务响应。写入结果不确定时，请先核对外部系统状态。
              </p>
              <div className="external-operation-list">
                {view?.invocations.map((inv, i) => (
                  <article className="external-operation" key={inv.id + i}>
                    <strong>
                      {outcomeLabels[inv.outcome] || inv.outcome} ·{' '}
                      {state.operations.find((op) => op.id === inv.operationId)?.name ||
                        inv.operationId}{' '}
                      v{inv.operationRevision}
                    </strong>
                    <small>
                      {new Date(inv.time).toLocaleString()} · 规则版本 {inv.stateRevision} ·{' '}
                      {inv.approved ? '经用户确认' : '策略判定'}
                    </small>
                    {inv.summary && <p>{inv.summary}</p>}
                    <details>
                      <summary>决策依据与请求摘要</summary>
                      <pre>
                        {json({
                          id: inv.id,
                          caller: inv.caller,
                          requestDigest: inv.requestDigest,
                          decision: inv.decision,
                        })}
                      </pre>
                    </details>
                  </article>
                ))}
              </div>
              <h3>配置变更</h3>
              {[...state.changes].reverse().map((change, i) => (
                <p key={i}>
                  v{change.revision} · {change.reason}
                  <small>
                    {new Date(change.time).toLocaleString()} · {change.actor}
                  </small>
                </p>
              ))}
            </>
          )}
        </fieldset>
      )}
    </div>
  );
}

function OperationEditor({
  value,
  onCancel,
  onSave,
}: {
  value: ExternalDefinition;
  onCancel: () => void;
  onSave: (op: ExternalDefinition, reason: string) => Promise<void>;
}) {
  const [op, setOp] = useState(structuredClone(value));
  const [input, setInput] = useState(json(value.inputSchema));
  const [output, setOutput] = useState(value.outputSchema ? json(value.outputSchema) : '');
  const [request, setRequest] = useState(json(value.request));
  const [reason, setReason] = useState('');
  const [error, setError] = useState('');
  async function save() {
    try {
      setError('');
      await onSave(
        {
          ...op,
          inputSchema: JSON.parse(input),
          outputSchema: output.trim() ? JSON.parse(output) : undefined,
          request: JSON.parse(request),
        },
        reason,
      );
    } catch (e) {
      setError(errorText(e));
    }
  }
  return (
    <div className="external-editor">
      <div className="form-row">
        <Field label="接口名称">
          <input value={op.name} onChange={(e) => setOp({ ...op, name: e.target.value })} />
        </Field>
        <Field label="启用状态">
          <select
            value={op.lifecycle}
            onChange={(e) => setOp({ ...op, lifecycle: e.target.value })}
          >
            <option value="draft">草稿</option>
            <option value="active">已审查，启用</option>
            <option value="disabled">停用</option>
          </select>
        </Field>
      </div>
      <Field label="说明">
        <textarea
          rows={2}
          value={op.description}
          onChange={(e) => setOp({ ...op, description: e.target.value })}
        />
      </Field>
      <div className="form-row">
        <Field label="实际效果">
          <select
            value={op.assessment.effects.access}
            onChange={(e) =>
              setOp({
                ...op,
                assessment: {
                  ...op.assessment,
                  effects: { ...op.assessment.effects, access: e.target.value as 'read' },
                },
              })
            }
          >
            <option value="unknown">待确认</option>
            <option value="read">只读</option>
            <option value="write">写入</option>
          </select>
        </Field>
        <Field label="基础风险">
          <select
            value={op.assessment.risk}
            onChange={(e) =>
              setOp({ ...op, assessment: { ...op.assessment, risk: e.target.value } })
            }
          >
            {Object.entries(riskLabels).map(([key, label]) => (
              <option key={key} value={key}>
                {label}
              </option>
            ))}
          </select>
        </Field>
      </div>
      <div className="external-checks">
        {Object.entries(effectLabels).map(([key, label]) => (
          <label className="check" key={key}>
            <input
              type="checkbox"
              checked={op.assessment.effects[key as keyof typeof effectLabels]}
              onChange={(e) =>
                setOp({
                  ...op,
                  assessment: {
                    ...op.assessment,
                    effects: { ...op.assessment.effects, [key]: e.target.checked },
                  },
                })
              }
            />
            {label}
          </label>
        ))}
      </div>
      <Field
        label="输入 Schema"
        hint="明确字段类型、必填项、枚举和上下限；额外字段会被拒绝。不支持的 Schema 约束不会被忽略。"
      >
        <textarea
          className="external-code"
          rows={8}
          value={input}
          onChange={(e) => setInput(e.target.value)}
        />
      </Field>
      <Field
        label="适配器请求模板"
        hint={
          op.kind === 'http'
            ? 'parameters 将输入字段映射到 path/query/body；bodyInput 可映射整个 JSON 请求体。'
            : op.kind === 'redis'
              ? 'redisCommand 使用受支持命令；redisParameters 列出按顺序绑定的字段，写入仅支持单键。'
              : 'query 为固定单条 SQL；sqlParameters 按顺序绑定字段。PostgreSQL 使用 $1 等占位符；MySQL 使用 ?，目前仅开放读取。'
        }
      >
        <textarea
          className="external-code"
          rows={7}
          value={request}
          onChange={(e) => setRequest(e.target.value)}
        />
      </Field>
      <Field label="输出 Schema（可选）" hint="HTTP JSON 响应可按此定义校验。">
        <textarea
          className="external-code"
          rows={3}
          value={output}
          onChange={(e) => setOutput(e.target.value)}
        />
      </Field>
      <div className="form-row">
        <Field label="数量参数路径（可选）" hint="例如 body.limit，受风险规则数量上限约束。">
          <input
            value={op.quantityArgument || ''}
            onChange={(e) => setOp({ ...op, quantityArgument: e.target.value })}
          />
        </Field>
        <Field label="尚未解决的问题" hint="问题未清空时接口不能执行。">
          <input
            value={op.blocked || ''}
            onChange={(e) => setOp({ ...op, blocked: e.target.value })}
          />
        </Field>
      </div>
      <Field label="审查依据 / 变更原因">
        <textarea
          rows={2}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          placeholder="已核对文档与参数范围，说明此次调整"
        />
      </Field>
      <ErrorBox error={error} />
      <div className="form-actions">
        <button className="button" onClick={onCancel}>
          取消编辑
        </button>
        <button
          className="button primary"
          disabled={!reason.trim() || !op.name.trim()}
          onClick={() => void save()}
        >
          保存接口版本
        </button>
      </div>
    </div>
  );
}

function PermissionsEditor({
  state,
  data,
  save,
}: {
  state: ExternalState;
  data: Catalog;
  save: (p: ExternalState['permissions'], grants: ExternalGrant[], reason: string) => Promise<void>;
}) {
  const [policy, setPolicy] = useState(structuredClone(state.permissions));
  const [grants, setGrants] = useState(structuredClone(state.grants));
  const [editing, setEditing] = useState<ExternalGrant>();
  const [reason, setReason] = useState('');
  return (
    <>
      <p className="external-hint">
        连接上限和单条授权必须同时覆盖请求。显式拒绝优先；多条不完整授权不会拼成更大的权限。SSH
        主体与连接使用的隧道主机分别管理。
      </p>
      <label className="check">
        <input
          type="checkbox"
          checked={policy.enabled}
          onChange={(e) => setPolicy({ ...policy, enabled: e.target.checked })}
        />
        启用此连接的业务调用
      </label>
      <h3>连接权限上限</h3>
      <div className="external-checks">
        {Object.entries(permissionLabels).map(([key, label]) => (
          <label className="check" key={key}>
            <input
              type="checkbox"
              checked={policy.ceiling.includes(key)}
              onChange={() => setPolicy({ ...policy, ceiling: toggle(policy.ceiling, key) })}
            />
            {label}
          </label>
        ))}
      </div>
      <details>
        <summary>显式禁止的接口</summary>
        {state.operations.map((op) => (
          <label className="check" key={op.id}>
            <input
              type="checkbox"
              checked={policy.deniedOperations.includes(op.id)}
              onChange={() =>
                setPolicy({ ...policy, deniedOperations: toggle(policy.deniedOperations, op.id) })
              }
            />
            {op.name}
          </label>
        ))}
      </details>
      <div className="external-toolbar">
        <h3>主体授权</h3>
        <button
          className="button small"
          onClick={() =>
            setEditing({
              id:
                globalThis.crypto?.randomUUID?.() ||
                'grant-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2),
              name: '',
              subject: { kind: 'local' },
              operations: {},
              permissions: ['read'],
              constraints: {},
              allowedFields: [],
              revoked: false,
              deny: false,
              autoRead: false,
              maxUses: 0,
              used: 0,
              grantedBy: '',
              updatedAt: '',
            })
          }
        >
          新增授权
        </button>
      </div>
      {editing ? (
        <GrantEditor
          key={editing.id}
          value={editing}
          state={state}
          data={data}
          onCancel={() => setEditing(undefined)}
          onSave={(grant) => {
            setGrants((items) => [...items.filter((g) => g.id !== grant.id), grant]);
            setEditing(undefined);
          }}
        />
      ) : (
        <div className="external-operation-list">
          {grants.map((g) => (
            <article className="external-operation" key={g.id}>
              <div className="external-toolbar">
                <strong>
                  {g.name} · {g.revoked ? '已撤销' : g.deny ? '显式拒绝' : '允许'}
                </strong>
                <div>
                  <button className="button small" onClick={() => setEditing(g)}>
                    编辑
                  </button>{' '}
                  <button
                    className="button small"
                    onClick={() =>
                      setGrants((items) =>
                        items.map((v) => (v.id === g.id ? { ...v, revoked: !v.revoked } : v)),
                      )
                    }
                  >
                    {g.revoked ? '恢复' : '撤销'}
                  </button>
                </div>
              </div>
              <p>
                {g.subject.kind === 'local'
                  ? '本机'
                  : g.subject.kind === 'host'
                    ? 'SSH 主机 ' +
                      (data.hosts.find((h) => h.id === g.subject.id)?.name || g.subject.id)
                    : '会话 ' + g.subject.id}{' '}
                · {g.permissions.map((p) => permissionLabels[p]).join('、')} ·{' '}
                {Object.keys(g.operations).length} 个接口版本
              </p>
              <small>
                {g.expiresAt ? '到期 ' + g.expiresAt : '无固定到期时间'} ·{' '}
                {g.maxUses ? `已用 ${g.used}/${g.maxUses} 次` : '不限制次数'}
                {Object.entries(g.operations).some(
                  ([id, rev]) =>
                    !state.operations.some((op) => op.id === id && op.revision === rev),
                )
                  ? ' · 引用旧接口版本，请编辑重新选择'
                  : ''}
              </small>
            </article>
          ))}
        </div>
      )}
      <Field label="权限变更原因">
        <input
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          placeholder="说明主体、接口或授权范围的调整"
        />
      </Field>
      <button
        className="button primary"
        disabled={!reason.trim() || !!editing}
        onClick={() => void save(policy, grants, reason)}
      >
        保存权限规则
      </button>
    </>
  );
}

function GrantEditor({
  value,
  state,
  data,
  onCancel,
  onSave,
}: {
  value: ExternalGrant;
  state: ExternalState;
  data: Catalog;
  onCancel: () => void;
  onSave: (g: ExternalGrant) => void;
}) {
  const [g, setG] = useState(structuredClone(value));
  const [constraints, setConstraints] = useState(json(value.constraints));
  const [fields, setFields] = useState(value.allowedFields.join(','));
  const [error, setError] = useState('');
  return (
    <div className="external-editor">
      <div className="form-row">
        <Field label="授权名称">
          <input value={g.name} onChange={(e) => setG({ ...g, name: e.target.value })} />
        </Field>
        <Field label="主体类型">
          <select
            value={g.subject.kind}
            onChange={(e) => setG({ ...g, subject: { kind: e.target.value, id: '' } })}
          >
            <option value="local">本机工具与桌面调用</option>
            <option value="host">指定 SSH 主机的会话</option>
            <option value="session">单个 SSH 会话</option>
          </select>
        </Field>
      </div>
      {g.subject.kind === 'host' && (
        <Field label="SSH 主机">
          <select
            value={g.subject.id}
            onChange={(e) => setG({ ...g, subject: { ...g.subject, id: e.target.value } })}
          >
            <option value="">请选择主机</option>
            {data.hosts.map((h) => (
              <option value={h.id} key={h.id}>
                {h.name}
              </option>
            ))}
          </select>
        </Field>
      )}
      {g.subject.kind === 'session' && (
        <Field
          label="会话标识"
          hint="使用 system.describe 返回的 sessionId。会话结束后此授权无法在新会话使用。"
        >
          <input
            value={g.subject.id}
            onChange={(e) => setG({ ...g, subject: { ...g.subject, id: e.target.value } })}
          />
        </Field>
      )}
      <h4>允许的能力</h4>
      <div className="external-checks">
        {Object.entries(permissionLabels).map(([key, label]) => (
          <label className="check" key={key}>
            <input
              type="checkbox"
              checked={g.permissions.includes(key)}
              onChange={() => setG({ ...g, permissions: toggle(g.permissions, key) })}
            />
            {label}
          </label>
        ))}
      </div>
      <h4>确切接口版本</h4>
      {state.operations.map((op) => (
        <label className="check" key={op.id}>
          <input
            type="checkbox"
            checked={g.operations[op.id] === op.revision}
            onChange={(e) => {
              const refs = { ...g.operations };
              if (e.target.checked) refs[op.id] = op.revision;
              else delete refs[op.id];
              setG({ ...g, operations: refs });
            }}
          />
          {op.name} v{op.revision} · {op.assessment.effects.access}
        </label>
      ))}
      <Field
        label="参数与对象范围（JSON）"
        hint={
          '字段路径支持嵌套，例如 {"body.tenant":{"values":["team-a"]},"key":{"prefix":"orders:"},"limit":{"maximum":20}}。'
        }
      >
        <textarea
          className="external-code"
          rows={4}
          value={constraints}
          onChange={(e) => setConstraints(e.target.value)}
        />
      </Field>
      <Field
        label="允许的输入字段（可选，逗号分隔）"
        hint="例如 body.name,body.status；留空使用接口 Schema 的字段范围。"
      >
        <input value={fields} onChange={(e) => setFields(e.target.value)} />
      </Field>
      <div className="form-row">
        <Field label="生效时间（RFC3339，可选）">
          <input
            value={g.validFrom || ''}
            onChange={(e) => setG({ ...g, validFrom: e.target.value })}
            placeholder="2026-09-26T09:00:00+08:00"
          />
        </Field>
        <Field label="到期时间（RFC3339，可选）">
          <input
            value={g.expiresAt || ''}
            onChange={(e) => setG({ ...g, expiresAt: e.target.value })}
          />
        </Field>
      </div>
      <Field label="最多使用次数（0 为不计次）">
        <input
          type="number"
          min={0}
          value={g.maxUses}
          onChange={(e) => setG({ ...g, maxUses: Number(e.target.value) })}
        />
      </Field>
      <Field label="限定精确请求摘要（可选）" hint="可从预览复制 requestDigest，配合一次授权使用。">
        <input
          value={g.requestDigest || ''}
          onChange={(e) => setG({ ...g, requestDigest: e.target.value })}
        />
      </Field>
      <label className="check">
        <input
          type="checkbox"
          checked={g.autoRead}
          onChange={(e) => setG({ ...g, autoRead: e.target.checked })}
        />
        允许已授权的低风险只读请求自动执行
      </label>
      <label className="check">
        <input
          type="checkbox"
          checked={g.deny}
          onChange={(e) => setG({ ...g, deny: e.target.checked })}
        />
        此项为显式拒绝规则
      </label>
      <ErrorBox error={error} />
      <div className="form-actions">
        <button className="button" onClick={onCancel}>
          取消
        </button>
        <button
          className="button"
          disabled={!g.name.trim()}
          onClick={() => {
            try {
              onSave({
                ...g,
                constraints: JSON.parse(constraints),
                allowedFields: fields
                  .split(',')
                  .map((s) => s.trim())
                  .filter(Boolean),
              });
            } catch (e) {
              setError(errorText(e));
            }
          }}
        >
          应用到待保存规则
        </button>
      </div>
    </div>
  );
}

function RiskEditor({
  value,
  save,
}: {
  value: RiskPolicy;
  save: (r: RiskPolicy, reason: string) => Promise<void>;
}) {
  const [risk, setRisk] = useState({ ...value });
  const [reason, setReason] = useState('');
  return (
    <>
      <p className="external-hint">
        风险规则不能授予调用权限。写入至少为中风险；删除、服务操作和权限变更至少为高风险。所有写入都需要逐次确认。
      </p>
      <div className="external-checks">
        {Object.entries({
          denySensitive: '禁止敏感数据操作',
          denyDestructive: '禁止删除或清空',
          denyCritical: '禁止严重风险操作',
          allowHigh: '允许高风险操作进入确认',
          confirmRead: '只读操作也逐次确认',
        }).map(([key, label]) => (
          <label key={key} className="check">
            <input
              type="checkbox"
              checked={risk[key as keyof RiskPolicy] as boolean}
              onChange={(e) => setRisk({ ...risk, [key]: e.target.checked })}
            />
            {label}
          </label>
        ))}
      </div>
      <Field label="生产环境写入风险下限">
        <select
          value={risk.productionFloor}
          onChange={(e) => setRisk({ ...risk, productionFloor: e.target.value })}
        >
          {['medium', 'high', 'critical'].map((v) => (
            <option key={v} value={v}>
              {riskLabels[v]}
            </option>
          ))}
        </select>
      </Field>
      <div className="form-row">
        {Object.entries({
          maxQuantity: '数量 / SQL 影响行数上限',
          maxRows: '结果行数上限（最多 100）',
          maxResponseBytes: '响应字节上限',
          timeoutSeconds: '执行超时（秒）',
        }).map(([key, label]) => (
          <Field key={key} label={label}>
            <input
              type="number"
              min={1}
              value={risk[key as keyof RiskPolicy] as number}
              onChange={(e) => setRisk({ ...risk, [key]: Number(e.target.value) })}
            />
          </Field>
        ))}
      </div>
      <Field label="风险规则变更原因">
        <input value={reason} onChange={(e) => setReason(e.target.value)} />
      </Field>
      <button
        className="button primary"
        disabled={!reason.trim()}
        onClick={() => void save(risk, reason)}
      >
        保存风险规则
      </button>
    </>
  );
}

function InvocationEditor({
  resource,
  state,
  data,
  run,
}: {
  resource: Resource;
  state: ExternalState;
  data: Catalog;
  run: (f: () => Promise<unknown>) => Promise<void>;
}) {
  const ops = state.operations.filter((op) => op.lifecycle === 'active');
  const [opId, setOpId] = useState(ops[0]?.id || '');
  const [args, setArgs] = useState('{}');
  const [subject, setSubject] = useState('local');
  const [session, setSession] = useState('');
  const [preview, setPreview] = useState<ExternalPreview>();
  const [result, setResult] = useState('');
  const op = ops.find((v) => v.id === opId);
  return (
    <>
      <p className="external-hint">
        预览只在本地检查定义和规则。SSH 主体用于模拟判定；实际执行来自本机会话，凭据由后端读取。
      </p>
      <Field label="调用接口">
        <select
          value={opId}
          onChange={(e) => {
            setOpId(e.target.value);
            setPreview(undefined);
          }}
        >
          {ops.map((op) => (
            <option key={op.id} value={op.id}>
              {op.name} v{op.revision}
            </option>
          ))}
        </select>
      </Field>
      {op && (
        <details>
          <summary>输入要求</summary>
          <pre>{json(op.inputSchema)}</pre>
        </details>
      )}
      <Field label="调用参数（JSON）">
        <textarea
          className="external-code"
          rows={6}
          value={args}
          onChange={(e) => {
            setArgs(e.target.value);
            setPreview(undefined);
          }}
        />
      </Field>
      <Field label="预览主体">
        <select
          value={subject}
          onChange={(e) => {
            setSubject(e.target.value);
            setPreview(undefined);
          }}
        >
          <option value="local">本机</option>
          {data.hosts.map((h) => (
            <option key={h.id} value={h.id}>
              模拟 SSH · {h.name}
            </option>
          ))}
        </select>
      </Field>
      {subject !== 'local' && (
        <Field label="模拟会话标识（可选）">
          <input
            value={session}
            onChange={(e) => {
              setSession(e.target.value);
              setPreview(undefined);
            }}
          />
        </Field>
      )}
      <button
        className="button"
        disabled={!op}
        onClick={() =>
          void run(async () => {
            setPreview(undefined);
            setResult('');
            const request = {
              resourceId: resource.id,
              action: 'system.invoke',
              operationId: op!.id,
              operationRevision: op!.revision,
              arguments: JSON.parse(args),
            };
            setPreview(
              subject === 'local'
                ? await api.PreviewExternalCall(request)
                : await api.SimulateExternalCall(request, {
                    hostId: subject,
                    sessionId: session,
                    local: false,
                  }),
            );
          })
        }
      >
        预览权限与风险
      </button>
      {preview && (
        <div className="external-preview">
          <strong>
            {preview.decision.action === 'deny'
              ? '拒绝执行'
              : preview.decision.action === 'ask'
                ? '需要确认本次请求'
                : '允许执行'}{' '}
            · 风险{riskLabels[preview.decision.risk]}
          </strong>
          <ul>
            {preview.decision.reasons.map((r, i) => (
              <li key={i}>{r}</li>
            ))}
          </ul>
          <pre>{json(preview.request)}</pre>
          <small>
            接口 v{preview.operation.revision} · 规则 v{preview.stateRevision}
          </small>
          <details>
            <summary>精确请求摘要</summary>
            <code>{preview.requestDigest}</code>
          </details>
          {subject === 'local' && preview.token && preview.decision.action !== 'deny' && (
            <button
              className="button primary"
              onClick={() =>
                void run(async () => {
                  const token = preview.token;
                  setPreview(undefined);
                  setResult(await api.InvokeExternalCall(token));
                })
              }
            >
              确认并执行此请求
            </button>
          )}
        </div>
      )}
      {result && (
        <div className="external-preview">
          <strong>执行结果</strong>
          <pre>
            {(() => {
              try {
                return json(JSON.parse(result));
              } catch {
                return result;
              }
            })()}
          </pre>
        </div>
      )}
    </>
  );
}
