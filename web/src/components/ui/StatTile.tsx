import clsx from "clsx";
import type { ReactNode } from "react";

interface StatTileProps {
  label: string;
  value: ReactNode;
  /** A short line under the number ("last 7 days"). */
  note?: string;
  className?: string;
}

/** One number with its name: the dashboard tiles. */
export function StatTile({ label, value, note, className }: StatTileProps) {
  return (
    <div className={clsx("rounded-card border border-line bg-bg p-3", className)}>
      <div className="text-sm text-muted">{label}</div>
      <div className="text-xl font-semibold">{value}</div>
      {note && <div className="text-xs text-muted">{note}</div>}
    </div>
  );
}
