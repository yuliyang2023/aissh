import { CSSProperties, useEffect, useId, useRef, useState } from 'react';

const storageKey = 'aissh.workbench.chat-width';
const defaultShare = 43;
const dividerWidth = 8;
const minPaneWidth = 280;

export function useWorkbenchSplit(enabled: boolean) {
  const ref = useRef<HTMLDivElement>(null);
  const chatId = useId();
  const [width, setWidth] = useState(0);
  const [share, setShare] = useState(() => {
    try {
      const stored = Number(localStorage.getItem(storageKey));
      if (Number.isFinite(stored) && stored > 0 && stored < 100) return stored;
    } catch {
      /* Storage may be unavailable. */
    }
    return defaultShare;
  });
  const [dragging, setDragging] = useState(false);
  const drag = useRef<{ pointerId: number; offset: number } | null>(null);
  const availableWidth = Math.max(minPaneWidth * 2, width - dividerWidth);
  const minimum = (minPaneWidth / availableWidth) * 100;
  const maximum = 100 - minimum;
  const clamp = (value: number) => Math.min(maximum, Math.max(minimum, value));
  const visibleShare = clamp(share);

  useEffect(() => {
    const element = ref.current;
    if (!element) return;
    const observer = new ResizeObserver(() => setWidth(element.clientWidth));
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  useEffect(() => {
    if (!enabled || width <= 640) {
      drag.current = null;
      setDragging(false);
    }
  }, [enabled, width]);
  useEffect(() => {
    if (dragging) return;
    try {
      localStorage.setItem(storageKey, String(share));
    } catch {
      /* Storage may be unavailable. */
    }
  }, [share, dragging]);

  const finishDrag = () => {
    drag.current = null;
    setDragging(false);
  };
  return {
    ref,
    chatId,
    dragging,
    style: {
      '--workbench-chat-share': `${share}fr`,
      '--workbench-ssh-share': `${100 - share}fr`,
    } as CSSProperties,
    divider: (
      <div
        className="workbench-divider"
        role="separator"
        tabIndex={0}
        aria-label="调整 AI 会话和 SSH 宽度"
        aria-orientation="vertical"
        aria-controls={chatId}
        aria-valuemin={Math.round(minimum)}
        aria-valuemax={Math.round(maximum)}
        aria-valuenow={Math.round(visibleShare)}
        aria-valuetext={`AI 会话 ${Math.round(visibleShare)}%，SSH ${Math.round(100 - visibleShare)}%`}
        title="拖动调整宽度，双击恢复默认；方向键微调"
        onPointerDown={(event) => {
          if (!enabled || width <= 640 || event.button !== 0 || !event.isPrimary) return;
          event.preventDefault();
          event.currentTarget.focus();
          event.currentTarget.setPointerCapture(event.pointerId);
          drag.current = {
            pointerId: event.pointerId,
            offset: event.clientX - event.currentTarget.getBoundingClientRect().left,
          };
          setDragging(true);
        }}
        onPointerMove={(event) => {
          if (!drag.current || event.pointerId !== drag.current.pointerId || !ref.current) return;
          const left = ref.current.getBoundingClientRect().left;
          setShare(clamp(((event.clientX - left - drag.current.offset) / availableWidth) * 100));
        }}
        onPointerUp={(event) => {
          if (event.currentTarget.hasPointerCapture(event.pointerId))
            event.currentTarget.releasePointerCapture(event.pointerId);
          finishDrag();
        }}
        onPointerCancel={finishDrag}
        onLostPointerCapture={finishDrag}
        onDoubleClick={() => setShare(defaultShare)}
        onKeyDown={(event) => {
          const step = event.shiftKey ? 10 : 2;
          const value = {
            ArrowLeft: visibleShare - step,
            ArrowRight: visibleShare + step,
            Home: minimum,
            End: maximum,
            Enter: defaultShare,
          }[event.key];
          if (value === undefined) return;
          event.preventDefault();
          setShare(clamp(value));
        }}
      />
    ),
  };
}
