import { FormEvent, RefObject, useEffect, useRef, useState } from 'react';
import {
  BookOpen,
  FileText,
  Table2,
  Plus,
  Search,
  X,
  Pencil,
  Trash2,
  ExternalLink,
  FolderOpen,
  Sparkles,
  Upload,
  LoaderCircle,
} from 'lucide-react';
import {
  allHosts,
  api,
  Catalog,
  connected,
  Conversation,
  errorText,
  Knowledge,
  newKnowledge,
} from './api';
import { Empty, ErrorBox, Field, Modal, Submit } from './components';
import { conversationKey } from './Conversations';
import { CreationChat, CreationTabs } from './CreationChat';

const types = { document: '文件文档', experience: '经验总结', data: '数据表' };
const icons = { document: FileText, experience: BookOpen, data: Table2 };
const dateText = (value: string) =>
  new Date(value).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
const normalizeDraft = (value: Knowledge) => ({
  ...newKnowledge(value.kind),
  ...value,
  tags: value.tags || [],
  columns: value.columns || [],
  rows: value.rows || [],
});
const related = (item: Knowledge, data: Catalog) =>
  [
    item.resourceId &&
      (data.resources.find((r) => r.id === item.resourceId)?.name || '已移除的系统'),
    item.hostId && (allHosts(data).find((h) => h.id === item.hostId)?.name || '已移除的主机'),
  ]
    .filter(Boolean)
    .join(' · ');

export function KnowledgeLibrary({
  data,
  conversations,
  searchRef,
  revision,
  intent,
  onConsumeIntent,
  onOpenConversation,
}: {
  data: Catalog;
  conversations: Conversation[];
  searchRef: RefObject<HTMLInputElement | null>;
  revision: number;
  intent: 'new' | Conversation | null;
  onConsumeIntent: () => void;
  onOpenConversation: (hostId: string, id: string) => void;
}) {
  const [items, setItems] = useState<Knowledge[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [search, setSearch] = useState('');
  const [kind, setKind] = useState<'all' | Knowledge['kind']>('all');
  const [editor, setEditor] = useState<Knowledge | null>(null);
  const [detail, setDetail] = useState<Knowledge | null>(null);
  const [deletion, setDeletion] = useState<Knowledge | null>(null);
  const [deleteError, setDeleteError] = useState('');
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState('');
  const [summary, setSummary] = useState<string | null>(null);
  const [summaryError, setSummaryError] = useState('');
  const [summaryBusy, setSummaryBusy] = useState(false);
  const request = useRef(0);
  async function reload() {
    const id = ++request.current;
    setLoading(true);
    try {
      const next = await api.ListKnowledge('');
      if (id === request.current) {
        setItems(next.map(normalizeDraft));
        setError('');
      }
    } catch (e) {
      if (id === request.current) setError(errorText(e));
    } finally {
      if (id === request.current) setLoading(false);
    }
  }
  useEffect(() => {
    void reload();
    return () => {
      request.current++;
    };
  }, [revision]);
  useEffect(() => {
    if (!intent) return;
    if (intent === 'new') setEditor(newKnowledge());
    else {
      setSummary(conversationKey(intent));
      setSummaryError('');
    }
    onConsumeIntent();
  }, [intent]);
  const query = search.trim().toLowerCase();
  const matches = (item: Knowledge) =>
    [
      item.title,
      item.content,
      item.filePath,
      item.source,
      item.sourceUrl,
      related(item, data),
      ...item.tags,
      ...item.columns,
      ...item.rows.flat(),
    ]
      .join(' ')
      .toLowerCase()
      .includes(query);
  const filtered = items.filter((item) => (kind === 'all' || item.kind === kind) && matches(item));
  async function importItem(table: boolean) {
    setBusy(true);
    setError('');
    try {
      const draft = table ? await api.PickKnowledgeTable() : await api.PickKnowledgeDocument();
      if (draft) setEditor(normalizeDraft(draft));
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  async function summarize() {
    const source = conversations.find((c) => conversationKey(c) === summary);
    if (!source) return;
    setSummaryBusy(true);
    setSummaryError('');
    try {
      const draft = await api.SummarizeConversation(source.hostId, source.id);
      setSummary(null);
      setEditor(normalizeDraft(draft));
    } catch (e) {
      setSummaryError(errorText(e));
    } finally {
      setSummaryBusy(false);
    }
  }
  async function remove() {
    if (!deletion) return;
    setBusy(true);
    setDeleteError('');
    try {
      await api.DeleteKnowledge(deletion.id, deletion.updatedAt);
      setDeletion(null);
      setNotice('知识条目已删除');
      await reload();
    } catch (e) {
      setDeleteError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="knowledge-library">
      <div className="knowledge-toolbar">
        <label className="search">
          <Search size={15} />
          <input
            ref={searchRef}
            aria-label="搜索本地知识"
            placeholder="搜索文档、经验或表格中的数据…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          {search && (
            <button className="icon-button" aria-label="清除知识搜索" onClick={() => setSearch('')}>
              <X size={12} />
            </button>
          )}
        </label>
        <div className="knowledge-actions">
          <button
            className="button"
            disabled={!connected() || busy}
            onClick={() => void importItem(false)}
          >
            <FolderOpen size={15} />
            添加文档
          </button>
          <button
            className="button"
            disabled={!connected() || busy}
            onClick={() => void importItem(true)}
          >
            <Upload size={15} />
            导入数据表
          </button>
          <button
            className="button"
            disabled={!connected() || busy || !conversations.some((c) => c.messageCount > 0)}
            onClick={() => {
              setSummary(
                conversations.find((c) => c.messageCount > 0)
                  ? conversationKey(conversations.find((c) => c.messageCount > 0)!)
                  : '',
              );
              setSummaryError('');
            }}
          >
            <Sparkles size={15} />
            从会话总结
          </button>
          <button
            className="button primary"
            disabled={!connected() || busy}
            onClick={() => setEditor(newKnowledge(kind === 'all' ? 'experience' : kind))}
          >
            <Plus size={15} />
            新建知识
          </button>
        </div>
      </div>
      <nav className="knowledge-filters" aria-label="知识分类">
        {(['all', 'document', 'experience', 'data'] as const).map((value) => (
          <button
            key={value}
            aria-current={kind === value ? 'page' : undefined}
            onClick={() => setKind(value)}
          >
            {value === 'all' ? '全部' : types[value]}
            <span>{items.filter((i) => value === 'all' || i.kind === value).length}</span>
          </button>
        ))}
      </nav>
      <p className="knowledge-intro">
        沉淀文档与排障经验，用自定义数据表管理资产、归属和其他 CMDB 资料。
      </p>
      <ErrorBox error={error} />
      {notice && (
        <div className="knowledge-notice" role="status">
          {notice}
          <button className="icon-button" aria-label="关闭知识提示" onClick={() => setNotice('')}>
            <X size={13} />
          </button>
        </div>
      )}
      {loading ? (
        <div className="loading">
          <LoaderCircle className="spin" />
          正在读取本地知识…
        </div>
      ) : error ? (
        <button className="button" onClick={() => void reload()}>
          重新加载
        </button>
      ) : filtered.length ? (
        <div className="table-card">
          <table className="knowledge-list">
            <thead>
              <tr>
                <th>知识条目</th>
                <th>关联与来源</th>
                <th>更新时间</th>
                <th className="align-right">操作</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((item) => {
                const Icon = icons[item.kind];
                return (
                  <tr key={item.id}>
                    <td>
                      <div className="identity">
                        <div className="row-icon blue">
                          <Icon size={19} />
                        </div>
                        <div>
                          <button className="knowledge-title" onClick={() => setDetail(item)}>
                            {item.title}
                          </button>
                          <small>
                            {types[item.kind]}
                            {item.kind === 'data' &&
                              ` · ${item.columns.length} 列 · ${item.rows.length} 条记录`}
                            {item.tags.length > 0 && ' · ' + item.tags.join('、')}
                          </small>
                        </div>
                      </div>
                    </td>
                    <td>
                      <span>{related(item, data) || '通用资料'}</span>
                      <small className="cell-small">
                        {item.sourceUrl
                          ? '在线来源 · 本地保存'
                          : item.conversationId
                            ? '由会话生成'
                            : item.filePath
                              ? '本机文档'
                              : '手动整理'}
                      </small>
                    </td>
                    <td>
                      <span className="subtle">{dateText(item.updatedAt)}</span>
                    </td>
                    <td>
                      <div className="row-actions">
                        <button className="button small" onClick={() => setDetail(item)}>
                          查看
                        </button>
                        <button
                          className="icon-button"
                          aria-label={'编辑 ' + item.title}
                          onClick={() => setEditor(item)}
                        >
                          <Pencil size={14} />
                        </button>
                        <button
                          className="icon-button danger-hover"
                          aria-label={'删除 ' + item.title}
                          onClick={() => {
                            setDeletion(item);
                            setDeleteError('');
                          }}
                        >
                          <Trash2 size={14} />
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : (
        <Empty
          icon={<BookOpen size={30} />}
          title={
            query
              ? '没有匹配的知识'
              : kind === 'all'
                ? '把资料和经验留在这里'
                : `还没有${types[kind]}`
          }
          description={
            query
              ? '可以搜索标题、标签、正文，以及数据表中的任意单元格。'
              : '添加本机或在线文档，从会话生成经验，或创建可以自由定义字段的数据表。'
          }
          action={
            !query && (
              <button
                className="button primary"
                disabled={!connected()}
                onClick={() => setEditor(newKnowledge(kind === 'all' ? 'data' : kind))}
              >
                <Plus size={16} />
                {kind === 'all' ? '创建数据表' : '新建' + types[kind]}
              </button>
            )
          }
        />
      )}
      <div className="knowledge-footnote">
        {filtered.length} 个条目 · 保存在本机{query && ' · 已筛选'}
      </div>
      {editor && (
        <KnowledgeEditor
          item={editor}
          data={data}
          onClose={() => setEditor(null)}
          onSaved={() => {
            setEditor(null);
            setNotice('已保存到本地知识库');
            void reload();
          }}
        />
      )}
      {detail && (
        <KnowledgeDetail
          item={detail}
          data={data}
          onClose={() => setDetail(null)}
          onEdit={() => {
            setEditor(detail);
            setDetail(null);
          }}
          onOpenConversation={() => {
            setDetail(null);
            onOpenConversation(detail.hostId, detail.conversationId);
          }}
        />
      )}
      {deletion && (
        <Modal
          title={'删除「' + deletion.title + '」？'}
          subtitle="删除本地知识条目及其中的数据。引用的原始文件和在线资料会保留。"
          onClose={() => !busy && setDeletion(null)}
        >
          <div className="modal-body">
            <ErrorBox error={deleteError} />
            <div className="form-actions">
              <button className="button" disabled={busy} onClick={() => setDeletion(null)}>
                取消
              </button>
              <button className="button danger" disabled={busy} onClick={() => void remove()}>
                确认删除
              </button>
            </div>
          </div>
        </Modal>
      )}
      {summary !== null && (
        <Modal
          title="从会话总结经验"
          subtitle="使用该主机的 AI 配置生成总结，编辑草稿后即可保存到本地知识库。"
          onClose={() => !summaryBusy && setSummary(null)}
        >
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void summarize();
            }}
          >
            <fieldset disabled={summaryBusy}>
              <Field label="来源会话">
                <select value={summary} required onChange={(e) => setSummary(e.target.value)}>
                  <option value="" disabled>
                    选择会话
                  </option>
                  {conversations
                    .filter((c) => c.messageCount > 0)
                    .map((c) => (
                      <option key={conversationKey(c)} value={conversationKey(c)}>
                        {allHosts(data).find((h) => h.id === c.hostId)?.name} · {c.title}
                      </option>
                    ))}
                </select>
              </Field>
              <p className="knowledge-intro">
                会话内容将交给该主机使用的 AI 服务，总结会保留来源与待确认事项。
              </p>
              <ErrorBox error={summaryError} />
              {summaryBusy && <p role="status">正在总结会话，请稍候…</p>}
              <div className="form-actions">
                <button className="button" type="button" onClick={() => setSummary(null)}>
                  取消
                </button>
                <Submit
                  busy={summaryBusy}
                  disabled={
                    !conversations.some((c) => conversationKey(c) === summary && c.messageCount > 0)
                  }
                  label="生成总结草稿"
                />
              </div>
            </fieldset>
          </form>
        </Modal>
      )}
    </div>
  );
}

function KnowledgeEditor({
  item,
  data,
  onClose,
  onSaved,
}: {
  item: Knowledge;
  data: Catalog;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [value, setValue] = useState(() => structuredClone(item));
  const [tags, setTags] = useState(item.tags.join('、'));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [creationMode, setCreationMode] = useState<'form' | 'chat'>('form');
  const [created, setCreated] = useState(false);
  const finish = () => {
    if (!busy) (created ? onSaved : onClose)();
  };
  const [paste, setPaste] = useState<string | null>(null);
  const [pasteError, setPasteError] = useState('');
  const [rowPage, setRowPage] = useState(0);
  const field = <K extends keyof Knowledge>(key: K, v: Knowledge[K]) =>
    setValue((old) => ({ ...old, [key]: v }));
  useEffect(() => {
    if (value.kind === 'data' && !value.columns.length)
      setValue((old) => ({ ...old, columns: ['名称', '说明'], rows: [] }));
  }, [value.kind]);
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      await api.SaveKnowledge({
        ...value,
        tags: tags
          .split(/[,，、]/)
          .map((t) => t.trim())
          .filter(Boolean),
      });
      onSaved();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  async function chooseFile() {
    setBusy(true);
    setError('');
    try {
      const draft = await api.PickKnowledgeDocument();
      if (draft)
        setValue((old) => ({
          ...old,
          filePath: draft.filePath,
          title: old.title || draft.title,
          content: draft.content || old.content,
        }));
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  async function applyPaste() {
    setBusy(true);
    setPasteError('');
    try {
      const draft = await api.ParseKnowledgeTable(paste || '');
      setValue((old) => ({ ...old, columns: draft.columns, rows: draft.rows }));
      setRowPage(0);
      setPaste(null);
    } catch (e) {
      setPasteError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  function setCell(row: number, col: number, text: string) {
    setValue((old) => ({
      ...old,
      rows: old.rows.map((r, i) => (i === row ? r.map((c, j) => (j === col ? text : c)) : r)),
    }));
  }
  const rowsPerPage = 30;
  const page = Math.min(rowPage, Math.max(0, Math.ceil(value.rows.length / rowsPerPage) - 1));
  return (
    <Modal
      wide
      className={creationMode === 'chat' ? 'creation-modal' : 'knowledge-editor'}
      title={item.id ? '编辑本地知识' : '新建本地知识'}
      subtitle="整理文档、经验和数据，并关联到对应系统或主机。"
      onClose={finish}
    >
      {!item.id && (
        <>
          <CreationTabs mode={creationMode} busy={busy} onChange={setCreationMode} />
          <CreationChat
            mode="knowledge"
            data={data}
            active={creationMode === 'chat'}
            onBusy={setBusy}
            onCreated={() => setCreated(true)}
            onFinish={finish}
          />
        </>
      )}
      <form onSubmit={save} hidden={creationMode === 'chat'}>
        <fieldset disabled={busy}>
          <div className="form-row">
            <Field label="标题">
              <input
                autoFocus
                required
                maxLength={200}
                value={value.title}
                onChange={(e) => field('title', e.target.value)}
                placeholder="例如：服务器资产台账"
              />
            </Field>
            <Field label="类型">
              <select
                value={value.kind}
                disabled={!!item.id || !!item.conversationId}
                onChange={(e) => field('kind', e.target.value as Knowledge['kind'])}
              >
                {Object.entries(types).map(([id, label]) => (
                  <option key={id} value={id}>
                    {label}
                  </option>
                ))}
              </select>
            </Field>
          </div>
          <div className="form-row">
            <Field label="关联系统">
              <select
                value={value.resourceId}
                onChange={(e) => field('resourceId', e.target.value)}
              >
                <option value="">通用资料</option>
                {value.resourceId && !data.resources.some((r) => r.id === value.resourceId) && (
                  <option value={value.resourceId}>已移除的系统</option>
                )}
                {data.resources.map((r) => (
                  <option key={r.id} value={r.id}>
                    {r.name}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="关联主机">
              <select
                value={value.hostId}
                onChange={(e) => field('hostId', e.target.value)}
                disabled={!!value.conversationId}
              >
                <option value="">不关联主机</option>
                {value.hostId && !allHosts(data).some((h) => h.id === value.hostId) && (
                  <option value={value.hostId}>已移除的主机</option>
                )}
                {allHosts(data).map((h) => (
                  <option key={h.id} value={h.id}>
                    {h.name}
                  </option>
                ))}
              </select>
            </Field>
          </div>
          <Field label="标签（可选）" hint="多个标签用逗号或顿号分隔。">
            <input
              value={tags}
              onChange={(e) => setTags(e.target.value)}
              placeholder="例如：生产环境、资产、运维"
            />
          </Field>
          {(value.kind === 'data' || value.kind === 'document') && (
            <Field
              label="在线来源链接（可选）"
              hint="关联在线 Excel、共享文档或 CMDB 页面，可从详情打开原始资料。"
            >
              <input
                type="url"
                value={value.sourceUrl}
                onChange={(e) => field('sourceUrl', e.target.value)}
                placeholder="https://…"
              />
            </Field>
          )}
          {value.kind === 'document' && (
            <Field
              label="本机文档（可选）"
              hint="保留原始文件位置；小于 256 KB 的 UTF-8 文本会带入正文，PDF 和 Office 文件通过原应用打开。"
            >
              <div className="input-action">
                <input
                  readOnly
                  value={value.filePath}
                  placeholder="选择本机文件，或填写上方在线链接"
                />
                <button type="button" className="button" onClick={() => void chooseFile()}>
                  <FolderOpen size={15} />
                  选择文件
                </button>
                {value.filePath && (
                  <button
                    type="button"
                    className="icon-button"
                    aria-label="移除文档引用"
                    onClick={() => field('filePath', '')}
                  >
                    <X size={14} />
                  </button>
                )}
              </div>
            </Field>
          )}
          {value.kind === 'data' && (
            <section className="knowledge-table-editor" aria-label="数据表编辑">
              <div className="knowledge-table-tools">
                <strong>字段与记录</strong>
                <button
                  type="button"
                  className="button small"
                  onClick={() => {
                    setPaste('');
                    setPasteError('');
                  }}
                >
                  <Upload size={14} />
                  粘贴 Excel 数据
                </button>
                <button
                  type="button"
                  className="button small"
                  disabled={value.columns.length >= 50}
                  onClick={() =>
                    setValue((old) => ({
                      ...old,
                      columns: [...old.columns, ''],
                      rows: old.rows.map((r) => [...r, '']),
                    }))
                  }
                >
                  <Plus size={14} />
                  添加字段
                </button>
                <button
                  type="button"
                  className="button small"
                  disabled={value.rows.length >= 5000}
                  onClick={() => {
                    field('rows', [...value.rows, value.columns.map(() => '')]);
                    setRowPage(Math.floor(value.rows.length / rowsPerPage));
                  }}
                >
                  <Plus size={14} />
                  添加记录
                </button>
              </div>
              <p className="knowledge-intro">
                字段可以自由命名。导入或粘贴的数据保存在本机，可在这里维护；在线来源更新后可重新导入。
              </p>
              <div className="knowledge-grid-scroll">
                <table className="knowledge-grid">
                  <thead>
                    <tr>
                      <th className="row-number">#</th>
                      {value.columns.map((column, col) => (
                        <th key={col}>
                          <div className="knowledge-column">
                            <input
                              aria-label={`第 ${col + 1} 列名称`}
                              required
                              value={column}
                              onChange={(e) =>
                                field(
                                  'columns',
                                  value.columns.map((v, i) => (i === col ? e.target.value : v)),
                                )
                              }
                              placeholder="字段名称"
                            />
                            <button
                              type="button"
                              className="icon-button"
                              aria-label={`删除第 ${col + 1} 列`}
                              disabled={value.columns.length <= 1}
                              onClick={() =>
                                setValue((old) => ({
                                  ...old,
                                  columns: old.columns.filter((_, i) => i !== col),
                                  rows: old.rows.map((r) => r.filter((_, i) => i !== col)),
                                }))
                              }
                            >
                              <X size={12} />
                            </button>
                          </div>
                        </th>
                      ))}
                      <th />
                    </tr>
                  </thead>
                  <tbody>
                    {value.rows
                      .slice(page * rowsPerPage, (page + 1) * rowsPerPage)
                      .map((row, offset) => {
                        const index = page * rowsPerPage + offset;
                        return (
                          <tr key={index}>
                            <td className="row-number">{index + 1}</td>
                            {row.map((cell, col) => (
                              <td key={col}>
                                <textarea
                                  rows={1}
                                  aria-label={`第 ${index + 1} 行 ${value.columns[col] || `第 ${col + 1} 列`}`}
                                  value={cell}
                                  onChange={(e) => setCell(index, col, e.target.value)}
                                />
                              </td>
                            ))}
                            <td>
                              <button
                                type="button"
                                className="icon-button danger-hover"
                                aria-label={`删除第 ${index + 1} 条记录`}
                                onClick={() =>
                                  field(
                                    'rows',
                                    value.rows.filter((_, i) => i !== index),
                                  )
                                }
                              >
                                <Trash2 size={13} />
                              </button>
                            </td>
                          </tr>
                        );
                      })}
                  </tbody>
                </table>
              </div>
              {!value.rows.length && (
                <p className="knowledge-intro">先定义字段，再添加记录或粘贴 Excel 中复制的表格。</p>
              )}
              <TablePagination
                count={value.rows.length}
                page={page}
                size={rowsPerPage}
                onPage={setRowPage}
              />
            </section>
          )}
          <Field label={value.kind === 'experience' ? '经验内容' : '说明与正文（可选）'}>
            <textarea
              className="knowledge-content-input"
              required={value.kind === 'experience'}
              rows={value.kind === 'data' ? 3 : 9}
              value={value.content}
              onChange={(e) => field('content', e.target.value)}
              placeholder={
                value.kind === 'experience'
                  ? '问题与环境、排查依据、处理步骤、结果和注意事项…'
                  : '用途、维护说明或可搜索的文档内容…'
              }
            />
          </Field>
          {value.source && (
            <div className="knowledge-provenance">
              <BookOpen size={14} />
              {value.source}
            </div>
          )}
          <ErrorBox error={error} />
          <div className="form-actions">
            <button type="button" className="button" onClick={finish}>
              取消
            </button>
            <Submit busy={busy} label="保存到本地知识库" />
          </div>
        </fieldset>
      </form>
      {paste !== null && (
        <Modal
          wide
          title="粘贴 Excel / CSV 数据"
          subtitle="复制在线 Excel 中的区域并粘贴到这里，第一行用作列名。应用后将替换当前草稿的数据表，保存后才会生效。"
          onClose={() => !busy && setPaste(null)}
        >
          <div className="modal-body">
            <Field label="表格内容">
              <textarea
                autoFocus
                rows={12}
                value={paste}
                disabled={busy}
                onChange={(e) => setPaste(e.target.value)}
                placeholder={'资产名称\t所属部门\t地址\n应用服务器\t研发部\t192.0.2.10'}
              />
            </Field>
            <ErrorBox error={pasteError} />
            <div className="form-actions">
              <button className="button" disabled={busy} onClick={() => setPaste(null)}>
                取消
              </button>
              <button
                className="button primary"
                disabled={busy || !paste.trim()}
                onClick={() => void applyPaste()}
              >
                应用表格
              </button>
            </div>
          </div>
        </Modal>
      )}
    </Modal>
  );
}

function TablePagination({
  count,
  page,
  size,
  onPage,
}: {
  count: number;
  page: number;
  size: number;
  onPage: (page: number) => void;
}) {
  return (
    <div className="knowledge-pagination">
      <span>
        {count} 条记录{count > size && ` · 第 ${page + 1} / ${Math.ceil(count / size)} 页`}
      </span>
      {count > size && (
        <>
          <button
            type="button"
            className="button small"
            disabled={page === 0}
            onClick={() => onPage(page - 1)}
          >
            上一页
          </button>
          <button
            type="button"
            className="button small"
            disabled={(page + 1) * size >= count}
            onClick={() => onPage(page + 1)}
          >
            下一页
          </button>
        </>
      )}
    </div>
  );
}

function KnowledgeDetail({
  item,
  data,
  onClose,
  onEdit,
  onOpenConversation,
}: {
  item: Knowledge;
  data: Catalog;
  onClose: () => void;
  onEdit: () => void;
  onOpenConversation: () => void;
}) {
  const [search, setSearch] = useState('');
  const [rowPage, setRowPage] = useState(0);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const rows = item.rows.filter((r) =>
    r.join(' ').toLowerCase().includes(search.trim().toLowerCase()),
  );
  async function open(source: boolean) {
    setBusy(true);
    setError('');
    try {
      if (source) await api.OpenKnowledgeSource(item.id);
      else await api.OpenKnowledgeDocument(item.id);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal
      wide
      title={item.title}
      subtitle={
        types[item.kind] +
        ' · ' +
        (related(item, data) || '通用资料') +
        ' · 更新于 ' +
        dateText(item.updatedAt)
      }
      onClose={onClose}
    >
      <div className="modal-body knowledge-detail">
        <div className="knowledge-detail-actions">
          {item.filePath && (
            <button className="button" disabled={busy} onClick={() => void open(false)}>
              <FolderOpen size={15} />
              打开本机文档
            </button>
          )}
          {item.sourceUrl && (
            <button className="button" disabled={busy} onClick={() => void open(true)}>
              <ExternalLink size={15} />
              打开在线来源
            </button>
          )}
          {item.conversationId && (
            <button className="button" onClick={onOpenConversation}>
              <BookOpen size={15} />
              查看来源会话
            </button>
          )}
          <button className="button" onClick={onEdit}>
            <Pencil size={15} />
            编辑
          </button>
        </div>
        <ErrorBox error={error} />
        {!!item.tags.length && (
          <div className="knowledge-tags">
            {item.tags.map((t) => (
              <span className="pill" key={t}>
                {t}
              </span>
            ))}
          </div>
        )}
        {item.filePath && <p className="knowledge-source">{item.filePath}</p>}
        {item.sourceUrl && <p className="knowledge-source">{item.sourceUrl}</p>}
        {item.kind === 'data' && (
          <>
            <label className="search">
              <Search size={14} />
              <input
                aria-label="搜索表内记录"
                placeholder="搜索任意字段中的内容…"
                value={search}
                onChange={(e) => {
                  setSearch(e.target.value);
                  setRowPage(0);
                }}
              />
            </label>
            <div className="knowledge-grid-scroll">
              <table className="knowledge-grid knowledge-read-grid">
                <thead>
                  <tr>
                    {item.columns.map((column) => (
                      <th key={column}>{column}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {rows.slice(rowPage * 50, (rowPage + 1) * 50).map((row, index) => (
                    <tr key={index}>
                      {row.map((cell, col) => (
                        <td key={col}>{cell || '—'}</td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {!rows.length && (
              <p className="knowledge-intro">
                {search ? '没有匹配的记录' : '还没有记录，点击编辑添加'}
              </p>
            )}
            <TablePagination count={rows.length} page={rowPage} size={50} onPage={setRowPage} />
          </>
        )}
        {item.content && <div className="knowledge-body">{item.content}</div>}
        {item.source && <div className="knowledge-provenance">{item.source}</div>}
        {!!item.changes?.length && (
          <details className="creation-audit">
            <summary>对话保存记录（{item.changes.length}）</summary>
            {item.changes
              .slice(-10)
              .reverse()
              .map((change, index) => (
                <div key={index}>
                  <small>{dateText(change.time)}</small>
                  <p>{change.quote}</p>
                  {change.values && (
                    <p>
                      {Object.entries(change.values)
                        .map(([key, value]) => key + '：' + value)
                        .join(' · ')}
                    </p>
                  )}
                </div>
              ))}
          </details>
        )}
      </div>
    </Modal>
  );
}
