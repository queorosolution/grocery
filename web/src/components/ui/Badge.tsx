import clsx from "clsx";
import type { ReactNode } from "react";
import type { Tone } from "./statusTone";

// Tinted background plus dark text keeps contrast above AA on the 12% tints.
const tones: Record<Tone, string> = {
  neutral: "bg-surface text-muted",
  info: "bg-[color-mix(in_srgb,var(--c-accent)_12%,white)] text-accent",
  success: "bg-[color-mix(in_srgb,var(--c-success)_12%,white)] text-success",
  warning: "bg-[color-mix(in_srgb,var(--c-warning)_12%,white)] text-warning",
  danger: "bg-[color-mix(in_srgb,var(--c-danger)_12%,white)] text-danger",
};

export function Badge({ tone = "neutral", children, className }: { tone?: Tone; children: ReactNode; className?: string }) {
  return (
    <span className={clsx("inline-flex h-6 items-center rounded-pill px-2 text-xs font-semibold", tones[tone], className)}>
      {children}
    </span>
  );
}
