import { useRef, type KeyboardEvent } from 'react';

export function useChatSubmit() {
  const composing = useRef(false);
  return {
    onCompositionStart: () => {
      composing.current = true;
    },
    onCompositionEnd: () => {
      composing.current = false;
    },
    onBlur: () => {
      composing.current = false;
    },
    onKeyDown: (event: KeyboardEvent<HTMLTextAreaElement>) => {
      if (
        event.key !== 'Enter' ||
        event.shiftKey ||
        composing.current ||
        event.nativeEvent.isComposing ||
        // WebKit may end composition before the confirming Enter keydown.
        event.nativeEvent.keyCode === 229
      )
        return;
      event.preventDefault();
      event.currentTarget.form?.requestSubmit();
    },
  };
}
