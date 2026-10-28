import clsx from "clsx";
import { t } from "@/lib/i18n";
import { formatLKR, saveCents } from "@/lib/money";

interface PriceProps {
  cents: number;
  /** Shop's printed price. Shown struck through when it is higher. */
  mrpCents?: number | null;
  showSave?: boolean;
  className?: string;
}

export function Price({ cents, mrpCents, showSave = false, className }: PriceProps) {
  const save = saveCents(cents, mrpCents);
  return (
    <span className={clsx("inline-flex flex-wrap items-baseline gap-x-2", className)}>
      <span className="font-semibold">{formatLKR(cents)}</span>
      {save > 0 && mrpCents != null && (
        <s className="text-sm text-muted">{formatLKR(mrpCents)}</s>
      )}
      {showSave && save > 0 && (
        <span className="text-sm font-semibold text-success">{t("price.save", { amount: formatLKR(save, { compact: true }) })}</span>
      )}
    </span>
  );
}
