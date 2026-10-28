import clsx from "clsx";
import { Minus, Plus } from "lucide-react";
import { t } from "@/lib/i18n";

interface QtyStepperProps {
  value: number;
  onChange: (qty: number) => void;
  min?: number;
  /** maxPerOrder from the API. The plus button stops here. */
  max?: number;
  className?: string;
}

const step = "inline-flex size-11 items-center justify-center rounded-pill text-ink hover:bg-surface disabled:text-muted disabled:hover:bg-transparent";

export function QtyStepper({ value, onChange, min = 0, max = Number.POSITIVE_INFINITY, className }: QtyStepperProps) {
  return (
    <div
      role="group"
      aria-label={t("qty.label")}
      className={clsx("inline-flex items-center rounded-pill border border-line bg-bg", className)}
    >
      <button type="button" className={step} aria-label={t("qty.decrease")} disabled={value <= min} onClick={() => onChange(value - 1)}>
        <Minus size={18} aria-hidden="true" />
      </button>
      <output aria-live="polite" className="min-w-8 text-center text-base font-semibold">{value}</output>
      <button type="button" className={step} aria-label={t("qty.increase")} disabled={value >= max} onClick={() => onChange(value + 1)}>
        <Plus size={18} aria-hidden="true" />
      </button>
    </div>
  );
}
