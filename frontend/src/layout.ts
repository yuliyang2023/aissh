import { useState } from 'react';

export function useStoredFlag(key: string, initial = false) {
  const [value, setValue] = useState(() => {
    try {
      const stored = localStorage.getItem(key);
      return stored === null ? initial : stored === 'true';
    } catch {
      return initial;
    }
  });
  return [
    value,
    (next: boolean) => {
      setValue(next);
      try {
        localStorage.setItem(key, String(next));
      } catch {
        /* Storage may be unavailable. */
      }
    },
  ] as const;
}
