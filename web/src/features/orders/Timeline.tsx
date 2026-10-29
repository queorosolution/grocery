import { t } from "@/lib/i18n";
import { formatDateTime } from "@/lib/time";
import { statusText } from "./statusText";
import type { OrderDetail } from "./types";

export function Timeline({ steps }: { steps: OrderDetail["timeline"] }) {
  return (
    <section aria-labelledby="timeline">
      <h2 id="timeline" className="mb-2 text-lg font-semibold">{t("order.timeline")}</h2>
      <ol className="flex flex-col gap-2 border-l border-line pl-4">
        {[...steps].reverse().map((s, i) => (
          <li key={`${s.status}-${s.at}`} className="relative">
            <span aria-hidden="true" className={`absolute -left-[21px] top-2 size-2.5 rounded-pill ${i === 0 ? "bg-accent" : "bg-line"}`} />
            <div className={i === 0 ? "font-semibold" : ""}>{t(statusText[s.status])}</div>
            <div className="text-sm text-muted">{formatDateTime(s.at)}</div>
          </li>
        ))}
      </ol>
    </section>
  );
}
