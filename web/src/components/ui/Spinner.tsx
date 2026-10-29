import clsx from "clsx";
import { t } from "@/lib/i18n";

export function Spinner({ className }: { className?: string }) {
  return (
    <span role="status" aria-label={t("common.loading")} className={clsx("inline-flex", className)}>
      <svg viewBox="0 0 24 24" className="size-5 animate-spin" fill="none" aria-hidden="true">
        <circle cx="12" cy="12" r="9" stroke="currentColor" strokeOpacity="0.25" strokeWidth="3" />
        <path d="M21 12a9 9 0 0 0-9-9" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
      </svg>
    </span>
  );
}
