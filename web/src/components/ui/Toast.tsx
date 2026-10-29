"use client";

import clsx from "clsx";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { t } from "@/lib/i18n";

type ToastTone = "neutral" | "danger";
interface ToastItem {
  id: number;
  message: string;
  tone: ToastTone;
}

interface ToastApi {
  show: (message: string, tone?: ToastTone) => void;
}

const ToastContext = createContext<ToastApi | null>(null);
const SHOW_MS = 4000;
const MAX_VISIBLE = 3;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const nextId = useRef(0);
  const timers = useRef(new Set<ReturnType<typeof setTimeout>>());

  const show = useCallback((message: string, tone: ToastTone = "neutral") => {
    const id = nextId.current++;
    setItems((list) => [...list.slice(-(MAX_VISIBLE - 1)), { id, message, tone }]);
    const timer = setTimeout(() => {
      timers.current.delete(timer);
      setItems((list) => list.filter((x) => x.id !== id));
    }, SHOW_MS);
    timers.current.add(timer);
  }, []);

  useEffect(() => {
    const pending = timers.current;
    return () => pending.forEach(clearTimeout);
  }, []);

  const api = useMemo(() => ({ show }), [show]);

  return (
    <ToastContext.Provider value={api}>
      {children}
      {/* One polite live region: a screen reader announces each new toast. */}
      <div aria-live="polite" aria-label={t("common.notice")}
        className="pointer-events-none fixed inset-x-4 bottom-4 z-50 flex flex-col items-center gap-2 md:inset-x-auto md:right-4 md:items-end">
        {items.map((x) => (
          <p key={x.id} className={clsx(
            "pointer-events-auto w-full max-w-sm rounded-input px-4 py-3 text-sm text-white shadow-raise md:w-auto",
            x.tone === "danger" ? "bg-danger" : "bg-ink",
          )}>
            {x.message}
          </p>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastApi {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast needs <ToastProvider>");
  return ctx;
}
