"use client";

import { useEffect, useRef } from "react";
import { playChime } from "@/lib/chime";

/** Plays the chime when a bill id shows up that the last answer did not have. The first answer never rings. */
export function useNewBillChime(ids: readonly string[] | undefined, on: boolean): void {
  const seen = useRef<Set<string> | null>(null);
  useEffect(() => {
    if (!ids) return;
    const before = seen.current;
    seen.current = new Set(ids);
    if (on && before && ids.some((id) => !before.has(id))) playChime();
  }, [ids, on]);
}
