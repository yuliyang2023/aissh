import { Check, ChevronRight, CircleHelp, TerminalSquare, X } from 'lucide-react';
import { ChatMessage } from './api';
import { MarkdownContent } from './MarkdownContent';
import { ChatContent } from './ChatContent';

const statuses: Record<string, string> = {
  pending: '待确认',
  running: '执行中',
  completed: '已完成',
  failed: '失败',
  rejected: '已拒绝',
  recovering: '等待恢复',
  unknown: '结果未确认',
  interrupted: '已中断',
};

export function ChatEntry({
  message: m,
  pending = false,
  active = true,
}: {
  message: ChatMessage;
  pending?: boolean;
  active?: boolean;
}) {
  const activity = m.activity;
  if (activity || m.proposal || m.role === 'system' || m.role === 'proposal') {
    const status = pending ? 'pending' : activity?.status;
    const running = status === 'running' || status === 'recovering';
    const title = activity?.title || m.proposal?.command || m.content;
    return (
      <details className={'chat-activity ' + (status || '')}>
        <summary>
          <ChevronRight size={13} className="activity-chevron" />
          {running ? (
            <span className="activity-spinner" />
          ) : status === 'completed' ? (
            <Check size={14} />
          ) : status === 'failed' ? (
            <X size={14} />
          ) : m.proposal || activity ? (
            <TerminalSquare size={14} />
          ) : (
            <CircleHelp size={14} />
          )}
          <span className="activity-title" title={title}>
            {title.replace(/\s+/g, ' ')}
          </span>
          <span className="activity-status">
            {status ? statuses[status] || status : m.proposal ? '命令记录' : '系统'}
          </span>
        </summary>
        <div className="activity-detail">
          {m.proposal ? (
            <>
              <MarkdownContent content={m.proposal.explanation} />
              <pre>{m.proposal.command}</pre>
              {m.proposal.expectedEffect && (
                <MarkdownContent content={'预期效果：' + m.proposal.expectedEffect} />
              )}
            </>
          ) : (
            <pre>{m.content}</pre>
          )}
          {activity?.detail && <pre>{activity.detail}</pre>}
        </div>
      </details>
    );
  }
  const streaming = m.role === 'assistant_stream';
  return (
    <article className={'chat-message ' + m.role} aria-busy={streaming}>
      <header>
        <strong>{m.role === 'user' ? '你' : m.role === 'error' ? '错误' : 'AI 助手'}</strong>
        <time>
          {new Date(m.time).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}
        </time>
        {streaming && <span className="stream-label">正在回复</span>}
      </header>
      <div className="chat-content">
        <ChatContent message={m} active={active} />
      </div>
    </article>
  );
}
