"use client";

import { useCallback, useRef, useSyncExternalStore } from "react";

// Polling that stops by itself (docs/08 section 5 rule 6): while the tab is hidden, once the
// thing is final, and 30 minutes after the page opened. Pass the result to `refetchInterval`.

const subscribe = (listener: () => void) => {
  document.addEventListener("visibilitychange", listener);
  return () => document.removeEventListener("visibilitychange", listener);
};
const isVisible = () => document.visibilityState === "visible";

export function usePolling(everyMs = 20_000, maxMs = 30 * 60_000): (active: boolean) => number | false {
  const visible = useSyncExternalStore(subscribe, isVisible, () => true);
  const openedAt = useRef<number | null>(null);
  return useCallback(
    (active: boolean) => {
      openedAt.current ??= Date.now();
      return active && visible && Date.now() - openedAt.current < maxMs ? everyMs : false;
    },
    [visible, everyMs, maxMs],
  );
}
