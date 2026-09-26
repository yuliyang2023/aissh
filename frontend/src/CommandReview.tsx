import { useState } from 'react';
import { Check, Copy, ShieldCheck } from 'lucide-react';
import { WorkbenchCommand, WorkbenchState } from './api';
import { ErrorBox, Modal } from './components';
import { commandRisk } from './risk';
import { MarkdownContent } from './MarkdownContent';

export function CommandReview({
  proposal,
  target,
  active,
  online,
  pending,
  busy,
  canApprove,
  approvalStatus,
  error,
  onAction,
}: {
  proposal: NonNullable<WorkbenchState['proposal']>;
  target: string;
  active: boolean;
  online: boolean;
  pending: boolean;
  busy: boolean;
  canApprove: boolean;
  approvalStatus: string;
  error: string;
  onAction: (command: WorkbenchCommand) => Promise<boolean>;
}) {
  const [open, setOpen] = useState(true);
  const [wrap, setWrap] = useState(true);
  const [copied, setCopied] = useState(false);
  const [copyError, setCopyError] = useState('');
  const risk = commandRisk(proposal.risk);
  const hint = !online
    ? '工作台连接中断，恢复连接后可处理命令'
    : approvalStatus ||
      (!canApprove ? '等待 SSH 回到空闲提示符后可执行' : '将在当前 SSH 会话中执行');

  async function copyCommand() {
    setCopyError('');
    try {
      await navigator.clipboard.writeText(proposal.command);
      setCopied(true);
    } catch {
      setCopyError('复制失败，请选中命令后手动复制');
    }
  }

  return (
    <>
      <section className={'native-approval ' + risk.level} aria-label="待审批命令">
        <div className="approval-summary">
          <ShieldCheck size={15} />
          <strong>待审计命令</strong>
          <span className="approval-risk">{risk.label}</span>
        </div>
        <code className="approval-preview" title={proposal.command}>
          {proposal.command.replace(/\s+/g, ' ')}
        </code>
        <button className="button small" onClick={() => setOpen(true)}>
          查看并审计
        </button>
      </section>
      {active && open && (
        <Modal
          title="命令审计"
          subtitle={'执行目标：' + target}
          className={'command-review ' + risk.level}
          onClose={() => setOpen(false)}
        >
          <div className="command-review-body">
            <div className="command-review-context">
              <span className="command-risk-badge">{risk.label}</span>
              {proposal.explanation && <MarkdownContent content={proposal.explanation} />}
              {proposal.expectedEffect && (
                <MarkdownContent content={'预期效果：' + proposal.expectedEffect} />
              )}
            </div>
            <div className="command-review-toolbar">
              <strong>完整命令</strong>
              <small>{Array.from(proposal.command).length.toLocaleString()} 字符</small>
              <label>
                <input type="checkbox" checked={wrap} onChange={(e) => setWrap(e.target.checked)} />
                自动换行
              </label>
              <button className="button small" onClick={() => void copyCommand()}>
                {copied ? <Check size={13} /> : <Copy size={13} />}
                {copied ? '已复制' : '复制命令'}
              </button>
            </div>
            <pre
              className={'command-review-code' + (wrap ? ' wrap' : '')}
              tabIndex={0}
              aria-label="完整命令"
            >
              <code>{proposal.command}</code>
            </pre>
            <ErrorBox error={copyError || error} />
          </div>
          <div className="command-review-footer">
            <p role="status">{hint}</p>
            <div className="command-review-actions">
              <button className="button" onClick={() => setOpen(false)}>
                稍后处理
              </button>
              <button
                className="button"
                disabled={!online || pending || busy}
                onClick={() => void onAction({ kind: 'reject', proposalId: proposal.id })}
              >
                拒绝
              </button>
              <button
                className="button primary"
                disabled={!online || pending || busy || !canApprove}
                onClick={() => void onAction({ kind: 'approve', proposalId: proposal.id })}
              >
                {pending ? '正在处理…' : '批准执行'}
              </button>
            </div>
          </div>
        </Modal>
      )}
    </>
  );
}
