import clsx from "clsx";
import { t } from "@/lib/i18n";
import { formatSlot } from "@/lib/time";
import type { Slot } from "./types";

interface Props {
  slots: readonly Slot[];
  value: string | null;
  onChange: (id: string) => void;
  error?: string;
}

/** Delivery windows of the next days. A full window is listed but cannot be picked. */
export function SlotPicker({ slots, value, onChange, error }: Props) {
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="mb-2 text-lg font-semibold">{t("checkout.slot")}</legend>
      {slots.length === 0 && <p className="text-muted">{t("checkout.noSlots")}</p>}
      {slots.map((s) => {
        const full = s.remaining <= 0;
        return (
          <label key={s.id} className={clsx("flex items-center gap-3 rounded-card border p-3", value === s.id ? "border-ink" : "border-line", full ? "opacity-60" : "cursor-pointer")}>
            <input type="radio" name="slot" disabled={full} checked={value === s.id} onChange={() => onChange(s.id)} className="size-4 accent-[var(--c-text)]" />
            <span className="flex-1">{formatSlot(s.startsAt, s.endsAt)}</span>
            <span className="text-sm text-muted">{full ? t("checkout.slotFull") : t("checkout.slotLeft", { count: s.remaining })}</span>
          </label>
        );
      })}
      {error && <p role="alert" className="text-sm text-danger">{error}</p>}
    </fieldset>
  );
}
