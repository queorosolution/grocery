"use client";

import clsx from "clsx";

export interface ChipItem {
  id: string;
  label: string;
}

interface ChipsProps {
  items: readonly ChipItem[];
  /** The selected chip id, or null for none (for example "All"). */
  value: string | null;
  onChange: (id: string | null) => void;
  label: string;
  className?: string;
}

/** A row of pills that scrolls sideways on a phone (category chips). Tap the chosen one again to clear. */
export function Chips({ items, value, onChange, label, className }: ChipsProps) {
  return (
    <div role="group" aria-label={label}
      className={clsx("flex gap-2 overflow-x-auto pb-1 [scrollbar-width:none]", className)}>
      {items.map((chip) => {
        const on = chip.id === value;
        return (
          <button
            key={chip.id}
            type="button"
            aria-pressed={on}
            onClick={() => onChange(on ? null : chip.id)}
            className={clsx(
              "h-9 shrink-0 whitespace-nowrap rounded-pill border px-4 text-sm",
              on ? "border-ink bg-ink font-semibold text-bg" : "border-line bg-bg text-ink hover:border-ink",
            )}
          >
            {chip.label}
          </button>
        );
      })}
    </div>
  );
}
