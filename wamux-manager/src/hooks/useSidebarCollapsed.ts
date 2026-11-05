import { useCallback, useState } from 'react';

const STORAGE_KEY = 'wamux.sidebar.collapsed';

function read(): boolean {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === '1';
  } catch {
    return false;
  }
}

/** Whether the desktop sidebar shows only icons, remembered in this browser. */
export function useSidebarCollapsed() {
  const [collapsed, setCollapsed] = useState(read);

  const toggle = useCallback(() => {
    setCollapsed((current) => {
      const next = !current;
      try {
        window.localStorage.setItem(STORAGE_KEY, next ? '1' : '0');
      } catch {
        // Private windows can refuse storage; the choice just lasts this visit.
      }
      return next;
    });
  }, []);

  return { collapsed, toggle };
}
