import { FormEvent, useEffect, useRef, useState } from 'react';
import { CheckCircle2, LoaderCircle, Send, Sparkles } from 'lucide-react';
import { api, Catalog, ChatMessage, CreationReceipt, errorText } from './api';
import { ErrorBox } from './components';

export function CreationTabs({
  mode,
  busy,
  onChange,
}: {
  mode: 'form' | 'chat';
  busy: boolean;
  onChange: (mode: 'form' | 'chat') => void;
}) {
  return (
    <div className="creation-tabs" aria-label="新建方式">
      <button
        type="button"
        className={mode === 'form' ? 'active' : ''}
        disabled={busy}
        aria-pressed={mode === 'form'}
        onClick={() => onChange('form')}
      >
        填写表单
      </button>
      <button
        type="button"
        className={mode === 'chat' ? 'active' : ''}
        disabled={busy}
        aria-pressed={mode === 'chat'}
        onClick={() => onChange('chat')}
      >
        <Sparkles size={15} />与 AI 对话创建
      </button>
    </div>
  );
}

export function CreationChat({
  mode,
  data,
  active,
  onBusy,
  onCreated,
  onFinish,
}: {
  mode: 'knowledge' | 'system';
  data: Catalog;
  active: boolean;
  onBusy: (busy: boolean) => void;
  onCreated: () => void;
  onFinish: () => void;
}) {
  const [conversationId] = useState(
    () =>
      globalThis.crypto?.randomUUID?.() ||
      'create-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2),
  );
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [receipts, setReceipts] = useState<CreationReceipt[]>([]);
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState('');
  const [error, setError] = useState('');
  const sending = useRef(false);
  const input = useRef<HTMLTextAreaElement>(null);
  const end = useRef<HTMLDivElement>(null);
  const profile = data.aiProfiles.find((p) => p.id === data.defaultAIProfileId);
  useEffect(
    () =>
      window.runtime?.EventsOn('catalog-create:' + conversationId, (message: string) =>
        setStatus(message),
      ),
    [conversationId],
  );
  useEffect(() => {
    if (active && !busy) input.current?.focus();
  }, [active, busy]);
  useEffect(() => {
    if (active) end.current?.scrollIntoView({ block: 'nearest' });
  }, [messages, receipts, active, status]);

  async function send(e?: FormEvent) {
    e?.preventDefault();
    const prompt = text.trim();
    if (!prompt || sending.current) return;
    sending.current = true;
    setBusy(true);
    onBusy(true);
    setError('');
    setStatus('AI 正在整理…');
    setText('');
    setMessages((old) => [
      ...old,
      { role: 'user', content: prompt, time: new Date().toISOString() },
    ]);
    try {
      const result = await api.AskCatalogAssistant({ mode, conversationId, prompt });
      setMessages(result.messages || []);
      setError(result.error || '');
      if (result.receipts?.length) {
        setReceipts((old) => [...old, ...result.receipts]);
        onCreated();
      }
    } catch (err) {
      setError(errorText(err));
      setText(prompt);
    } finally {
      sending.current = false;
      setBusy(false);
      onBusy(false);
      setStatus('');
    }
  }
  return (
    <div className="creation-chat" hidden={!active}>
      <div className="creation-intro">
        <Sparkles size={20} />
        <div>
          <strong>
            {mode === 'knowledge' ? '描述信息，自动整理并保存' : '说说你要添加的系统'}
          </strong>
          <p>
            {mode === 'knowledge'
              ? '支持资产台账、业务数据和经验。AI 会查找已有资料，补充记录或建立合适的数据表。'
              : '提供名称、类型和服务地址，AI 会补齐必要信息并保存系统。凭据可在创建后绑定。'}
          </p>
          <small>使用 {profile?.name || '本机 Codex'} · 保存到本机</small>
        </div>
      </div>
      <div className="creation-messages" role="log" aria-label="创建对话" aria-live="polite">
        {messages.map((message, index) => (
          <div key={index} className={'creation-message ' + message.role}>
            <span>
              {message.role === 'user'
                ? '你'
                : message.role === 'system'
                  ? '保存结果'
                  : message.role === 'error'
                    ? '提示'
                    : 'AI'}
            </span>
            <p>{message.content}</p>
          </div>
        ))}
        {receipts.length > 0 && (
          <div className="creation-receipts" aria-label="已保存的资料">
            {receipts.map((receipt, index) => (
              <div key={index} className="creation-receipt">
                <CheckCircle2 size={16} />
                <div>
                  <strong>{receipt.detail}</strong>
                  <small>
                    {receipt.status === 'unchanged'
                      ? '已存在，无需重复创建'
                      : receipt.status === 'updated'
                        ? '已更新'
                        : '已保存'}
                  </small>
                </div>
              </div>
            ))}
          </div>
        )}
        {busy && (
          <div className="creation-status" role="status">
            <LoaderCircle size={15} className="spin" />
            {status}
          </div>
        )}
        <div ref={end} />
      </div>
      <ErrorBox error={error} />
      <form className="creation-composer" onSubmit={send}>
        <label htmlFor={'creation-input-' + conversationId}>告诉 AI 要保存的信息</label>
        <textarea
          ref={input}
          id={'creation-input-' + conversationId}
          value={text}
          rows={3}
          disabled={busy}
          maxLength={5000}
          onChange={(e) => setText(e.target.value)}
          placeholder={
            mode === 'knowledge'
              ? '例如：192.0.2.10 属于研发部，负责人是张三'
              : '例如：添加“资产管理”系统，类型是 HTTP，地址是 https://cmdb.example.com'
          }
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault();
              void send();
            }
          }}
        />
        <div className="form-actions">
          <small>明确的信息会自动保存 · Shift + Enter 换行</small>
          <button type="button" className="button" disabled={busy} onClick={onFinish}>
            {receipts.length ? '完成' : '关闭'}
          </button>
          <button className="button primary" type="submit" disabled={busy || !text.trim()}>
            {busy ? <LoaderCircle size={15} className="spin" /> : <Send size={15} />}
            {busy ? '处理中' : '发送'}
          </button>
        </div>
      </form>
    </div>
  );
}
