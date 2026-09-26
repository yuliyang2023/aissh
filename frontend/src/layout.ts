import { useEffect, useState } from 'react';

export function useStoredStringSet(key: string) {
  const [value, setValue] = useState<Set<string>>(() => {
    try {
      const stored: unknown = JSON.parse(localStorage.getItem(key) || '[]');
      return new Set(
        Array.isArray(stored)
          ? stored.filter((item): item is string => typeof item === 'string')
          : [],
      );
    } catch {
      return new Set();
    }
  });
  useEffect(() => {
    try {
      localStorage.setItem(key, JSON.stringify([...value]));
    } catch {
      /* Storage may be unavailable. */
    }
  }, [key, value]);
  return [value, setValue] as const;
}

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
