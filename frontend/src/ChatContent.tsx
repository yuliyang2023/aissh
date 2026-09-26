import { useState } from 'react';
import { createPortal } from 'react-dom';
import { ChatMessage } from './api';
import { CommandContent } from './CommandContent';
import { MarkdownContent } from './MarkdownContent';
import { Modal } from './components';

// Recognize the persisted approval receipts so existing conversations also get
// the compact presentation. Keep the command verbatim for viewing and copying.
const approvalPrefixes = ['已批准执行命令：', 'Approved command: '];

export function ChatContent({
  message,
  active = true,
}: {
  message: ChatMessage;
  active?: boolean;
}) {
  if (message.role === 'assistant' || message.role === 'assistant_stream') {
    return <MarkdownContent content={message.content} />;
  }
  const prefix =
    message.role === 'user' && approvalPrefixes.find((value) => message.content.startsWith(value));
  if (prefix && message.content.length > prefix.length) {
    return (
      <ApprovedCommand
        key={message.content}
        command={message.content.slice(prefix.length)}
        active={active}
      />
    );
  }
  return <>{message.content}</>;
}

function ApprovedCommand({ command, active }: { command: string; active: boolean }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <div className="chat-command-approval">
        <span>已批准执行命令</span>
        <button type="button" className="button small" onClick={() => setOpen(true)}>
          查看命令
        </button>
      </div>
      {active &&
        open &&
        createPortal(
          <Modal title="已批准的命令" className="command-review" onClose={() => setOpen(false)}>
            <div className="command-review-body">
              <CommandContent command={command} />
            </div>
          </Modal>,
          document.body,
        )}
    </>
  );
}
