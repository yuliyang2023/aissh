import { useState } from 'react';
import { Check, Copy } from 'lucide-react';
import { ErrorBox } from './components';
import { ShellCode } from './ShellCode';

export function CommandContent({ command, error = '' }: { command: string; error?: string }) {
  const [wrap, setWrap] = useState(true);
  const [copied, setCopied] = useState(false);
  const [copyError, setCopyError] = useState('');

  async function copyCommand() {
    setCopyError('');
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
    } catch {
      setCopyError('复制失败，请选中命令后手动复制');
    }
  }

  return (
    <>
      <div className="command-review-toolbar">
        <strong>完整命令</strong>
        <span className="command-language">Shell</span>
        <small>{Array.from(command).length.toLocaleString()} 字符</small>
        <label>
          <input type="checkbox" checked={wrap} onChange={(e) => setWrap(e.target.checked)} />
          自动换行
        </label>
        <button type="button" className="button small" onClick={() => void copyCommand()}>
          {copied ? <Check size={13} /> : <Copy size={13} />}
          {copied ? '已复制' : '复制命令'}
        </button>
      </div>
      <ShellCode command={command} wrap={wrap} />
      <ErrorBox error={copyError || error} />
    </>
  );
}
