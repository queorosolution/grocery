import clsx from "clsx";

/** A grey placeholder block. Size it with className (h-4 w-24). */
export function Skeleton({ className }: { className?: string }) {
  return <div aria-hidden="true" className={clsx("animate-pulse rounded-input bg-surface", className)} />;
}
