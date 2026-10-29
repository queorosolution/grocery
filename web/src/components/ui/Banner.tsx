import clsx from "clsx";
import { X } from "lucide-react";
import type { ReactNode } from "react";
import { t } from "@/lib/i18n";
import { IconButton } from "./IconButton";

type BannerTone = "info" | "success" | "warning" | "danger";

const tones: Record<BannerTone, string> = {
  info: "border-line bg-surface",
  success: "border-success bg-[color-mix(in_srgb,var(--c-success)_8%,white)]",
  warning: "border-warning bg-[color-mix(in_srgb,var(--c-warning)_8%,white)]",
  danger: "border-danger bg-[color-mix(in_srgb,var(--c-danger)_8%,white)]",
};

interface BannerProps {
  tone?: BannerTone;
  children: ReactNode;
  onDismiss?: () => void;
  className?: string;
}

export function Banner({ tone = "info", children, onDismiss, className }: BannerProps) {
  return (
    // Errors interrupt a screen reader; everything else waits its turn.
    <div role={tone === "danger" ? "alert" : "status"}
      className={clsx("flex items-start gap-2 rounded-input border py-3 pl-3 text-sm", onDismiss ? "pr-1" : "pr-3", tones[tone], className)}>
      <div className="flex-1">{children}</div>
      {onDismiss && <IconButton aria-label={t("common.dismiss")} icon={<X size={18} />} onClick={onDismiss} className="-my-2" />}
    </div>
  );
}
