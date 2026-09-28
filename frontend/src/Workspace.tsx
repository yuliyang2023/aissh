import { useState } from 'react';
import { FolderOpen } from 'lucide-react';
import { api, errorText } from './api';
import { ErrorBox, Field, Modal, Submit } from './components';

export function WorkspaceField({
  value,
  onChange,
  disabled = false,
  hint = 'AI 在本机读取项目的目录，默认只读。留空关闭本机项目访问。',
}: {
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  hint?: string;
}) {
  const [error, setError] = useState('');
  const [picking, setPicking] = useState(false);
  return (
    <div>
      <Field label="AI 工作目录（本机）" hint={hint}>
        <div className="input-action">
          <input
            aria-label="AI 工作目录（本机）"
            value={value}
            disabled={disabled || picking}
            onChange={(event) => onChange(event.target.value)}
            placeholder="选择文件夹或输入本机路径"
          />
          <button
            className="button"
            type="button"
            aria-label="选择工作目录"
            title="选择工作目录"
            disabled={disabled || picking}
            onClick={async () => {
              setPicking(true);
              setError('');
              try {
                const directory = await api.PickWorkspace();
                if (directory) onChange(directory);
              } catch (error) {
                setError(errorText(error));
              } finally {
                setPicking(false);
              }
            }}
          >
            <FolderOpen size={16} />
          </button>
        </div>
      </Field>
      <ErrorBox error={error} />
    </div>
  );
}

export function WorkspaceEditor({
  workspace,
  busy,
  blocked,
  error,
  onSave,
  onClose,
}: {
  workspace: string;
  busy: boolean;
  blocked: boolean;
  error: string;
  onSave: (directory: string) => Promise<boolean>;
  onClose: () => void;
}) {
  const [value, setValue] = useState(workspace);
  return (
    <Modal
      title="切换 AI 工作目录"
      subtitle="对当前对话生效，下次连接时保留。AI 后续请求会使用此目录。"
      onClose={() => !busy && onClose()}
    >
      <form
        onSubmit={async (event) => {
          event.preventDefault();
          if (!blocked && (await onSave(value))) onClose();
        }}
      >
        <WorkspaceField value={value} onChange={setValue} disabled={blocked} />
        <ErrorBox error={error} />
        <div className="form-actions">
          <button className="button" type="button" disabled={blocked} onClick={() => setValue('')}>
            清空目录
          </button>
          <button className="button" type="button" disabled={busy} onClick={onClose}>
            取消
          </button>
          <Submit label="应用到当前对话" busy={busy} disabled={blocked} />
        </div>
      </form>
    </Modal>
  );
}
