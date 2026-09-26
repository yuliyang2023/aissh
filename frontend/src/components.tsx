import { ReactNode, useLayoutEffect, useRef } from 'react';
import { X, LoaderCircle } from 'lucide-react';

const openModals: HTMLDialogElement[] = [];

export function Modal({
  title,
  subtitle,
  children,
  onClose,
  wide = false,
  className = '',
}: {
  title: string;
  subtitle?: string;
  children: ReactNode;
  onClose: () => void;
  wide?: boolean;
  className?: string;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useLayoutEffect(() => {
    const el = ref.current!;
    el.showModal();
    openModals.push(el);
    return () => {
      openModals.splice(openModals.indexOf(el), 1);
      // Close while connected to allow the browser to restore focus.
      el.close();
    };
  }, []);
  useLayoutEffect(() => {
    const el = ref.current!;
    const handleEscape = (event: KeyboardEvent) => {
      if (
        event.key !== 'Escape' ||
        event.isComposing ||
        event.defaultPrevented ||
        openModals[openModals.length - 1] !== el
      )
        return;
      // Capture even when focus has left the dialog, before native window actions.
      event.preventDefault();
      event.stopImmediatePropagation();
      if (!event.repeat) onClose();
    };
    window.addEventListener('keydown', handleEscape, true);
    return () => window.removeEventListener('keydown', handleEscape, true);
  }, [onClose]);
  return (
    <dialog
      ref={ref}
      aria-label={title}
      className={'modal ' + (wide ? 'wide ' : '') + className}
      onCancel={(e) => {
        e.preventDefault();
        e.stopPropagation();
        onClose();
      }}
    >
      <div className="modal-heading">
        <div>
          <h2>{title}</h2>
          {subtitle && <p>{subtitle}</p>}
        </div>
        <button type="button" className="icon-button" aria-label="关闭窗口" onClick={onClose}>
          <X size={19} />
        </button>
      </div>
      {children}
    </dialog>
  );
}
export function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
      {hint && <small>{hint}</small>}
    </label>
  );
}
export function ErrorBox({ error }: { error: string }) {
  return error ? (
    <div className="error-box" role="alert">
      {error}
    </div>
  ) : null;
}
export function Submit({
  busy,
  label = '保存',
  disabled = false,
}: {
  busy: boolean;
  label?: string;
  disabled?: boolean;
}) {
  return (
    <button className="button primary" type="submit" disabled={busy || disabled}>
      {busy && <LoaderCircle size={15} className="spin" />}
      {busy ? '正在处理…' : label}
    </button>
  );
}
export function Empty({
  icon,
  title,
  description,
  action,
}: {
  icon: ReactNode;
  title: string;
  description: string;
  action?: ReactNode;
}) {
  return (
    <div className="empty">
      <div className="empty-icon">{icon}</div>
      <h3>{title}</h3>
      <p>{description}</p>
      {action}
    </div>
  );
}
