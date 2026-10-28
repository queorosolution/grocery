"use client";

import clsx from "clsx";
import { X } from "lucide-react";
import { useEffect, useId, useRef, type ReactNode, type SyntheticEvent } from "react";
import { t } from "@/lib/i18n";
import { IconButton } from "./IconButton";

export interface DialogProps {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  /** Buttons pinned below the content. */
  footer?: ReactNode;
}

// Both shapes sit on the native <dialog>: the browser traps focus, closes on Esc and
// gives focus back to the button that opened it.
const shapes = {
  modal: "m-auto w-[calc(100%-32px)] max-w-md rounded-card",
  // Phone: a sheet rising from the bottom. Desktop: a panel on the right.
  sheet: clsx(
    "m-0 mt-auto max-h-[85dvh] w-full max-w-none rounded-t-card",
    "md:ml-auto md:mt-0 md:h-dvh md:max-h-none md:w-[420px] md:rounded-none",
  ),
} as const;

export function Dialog({ shape, open, onClose, title, children, footer }: DialogProps & { shape: keyof typeof shapes }) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();

  useEffect(() => {
    const dialog = ref.current;
    if (!open || !dialog) return;
    dialog.showModal();
    const page = document.documentElement;
    const before = page.style.overflow;
    page.style.overflow = "hidden";
    return () => {
      page.style.overflow = before;
      if (dialog.open) dialog.close();
    };
  }, [open]);

  if (!open) return null;

  const cancel = (e: SyntheticEvent) => {
    e.preventDefault(); // Esc: let the parent decide, so state stays in one place.
    onClose();
  };

  return (
    <dialog
      ref={ref}
      aria-labelledby={titleId}
      onCancel={cancel}
      onClick={(e) => e.target === e.currentTarget && onClose()}
      className={clsx("flex flex-col overflow-hidden bg-bg p-0 text-ink shadow-raise backdrop:bg-black/40", shapes[shape])}
    >
      <header className="flex items-center justify-between gap-2 border-b border-line py-1 pl-4 pr-1">
        <h2 id={titleId} className="text-lg font-semibold">{title}</h2>
        <IconButton aria-label={t("common.close")} icon={<X size={20} />} onClick={onClose} />
      </header>
      <div className="min-h-0 flex-1 overflow-y-auto p-4">{children}</div>
      {footer && <footer className="flex gap-2 border-t border-line p-3">{footer}</footer>}
    </dialog>
  );
}
