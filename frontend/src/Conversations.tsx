import { useEffect, useState } from 'react';
import { ArrowUpRight, MessageSquare, Plus, Sparkles, TerminalSquare } from 'lucide-react';
import { api, ChatMessage, Conversation, errorText, Host } from './api';
import { Empty, ErrorBox } from './components';
import { Monitor } from './Monitor';
import { sessionAddress } from './api';
import { commandRisk } from './risk';

export const conversationKey = (c: Pick<Conversation, 'hostId' | 'id'>) => c.hostId + '/' + c.id;
const roles: Record<string, string> = {
  user: '你',
  assistant: 'AI 助手',
  proposal: '历史命令建议',
  system: '系统',
  error: '错误',
};
export function Conversations({
  hosts,
  current,
  busy,
  onNew,
  onContinue,
}: {
  hosts: Host[];
  current?: Conversation;
  busy: boolean;
  onNew: (hostId?: string) => void;
  onContinue: (conversation: Conversation) => void;
}) {
  const host = hosts.find((h) => h.id === current?.hostId);
  const key = current ? conversationKey(current) : '';
  const [detail, setDetail] = useState<{ key: string; messages: ChatMessage[]; error: string }>();
  useEffect(() => {
    if (!current) return;
    let disposed = false;
    void api
      .GetConversation(current.hostId, current.id)
      .then((messages) => {
        if (!disposed) setDetail({ key, messages, error: '' });
      })
      .catch((e) => {
        if (!disposed) setDetail({ key, messages: [], error: errorText(e) });
      });
    return () => {
      disposed = true;
    };
  }, [key, current?.updatedAt]);
  return (
    <div className="native-workbench history-workbench">
      <Monitor
        panels={[]}
        online={false}
        pending={false}
        now={Date.now()}
        onAction={async () => false}
      />
      <section className="native-chat" aria-label="会话历史">
        <header className="native-chat-heading">
          <Sparkles size={17} />
          <div>
            <strong>AI 对话</strong>
            <small>{current ? '本机保存的对话记录' : '和 AI 一起处理服务器问题'}</small>
          </div>
        </header>
        <div className="native-chat-messages" key={key}>
          {current ? (
            detail?.key !== key ? (
              <p className="subtle">正在读取对话…</p>
            ) : detail.error ? (
              <ErrorBox error={detail.error} />
            ) : detail.messages.filter((m) => m.role !== 'system').length ? (
              detail.messages.map((m, i) => (
                <article key={i} className={'chat-message ' + m.role}>
                  <header>
                    <strong>{roles[m.role] || '系统'}</strong>
                    <time>
                      {new Date(m.time).toLocaleString('zh-CN', {
                        month: '2-digit',
                        day: '2-digit',
                        hour: '2-digit',
                        minute: '2-digit',
                      })}
                    </time>
                  </header>
                  <div className="chat-content">{m.content}</div>
                  {m.proposal && (
                    <div className="chat-proposal">
                      <small>历史记录 · {commandRisk(m.proposal.risk).label}</small>
                      <pre>{m.proposal.command}</pre>
                    </div>
                  )}
                </article>
              ))
            ) : (
              <Empty
                icon={<Sparkles size={26} />}
                title="开始一个新话题"
                description="连接服务器后，向 AI 描述你要处理的问题。"
              />
            )
          ) : (
            <Empty
              icon={<MessageSquare size={28} />}
              title="从一个会话开始"
              description="新建会话连接服务器，或在左侧选择已有对话。"
              action={
                <button className="button primary" disabled={busy} onClick={() => onNew()}>
                  <Plus size={15} />
                  新建会话
                </button>
              }
            />
          )}
        </div>
        <div className="history-footer">
          <Sparkles size={14} />
          <span>
            {current
              ? '连接后可继续提问，已有上下文会自动恢复。'
              : 'AI 对话、SSH 和监控会在同一个工作区打开。'}
          </span>
        </div>
      </section>
      <div className="native-ssh">
        <div className="disconnected-terminal">
          <Empty
            icon={<TerminalSquare size={28} />}
            title={host ? host.name : 'SSH 终端'}
            description={host ? sessionAddress(host) : '连接后显示终端输出和服务器监控'}
            action={
              current && (
                <button
                  className="button primary"
                  disabled={busy}
                  onClick={() => onContinue(current)}
                >
                  {current.messageCount ? '连接并继续' : '连接并开始'}
                  <ArrowUpRight size={15} />
                </button>
              )
            }
          />
        </div>
      </div>
    </div>
  );
}
