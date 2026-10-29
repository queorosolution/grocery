import clsx from "clsx";
import { Banner } from "@/components/ui";
import { t } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import { diffLines, type LineChange } from "./revisionDiff";
import type { OrderDetail, OrderLine } from "./types";

const text = (l: OrderLine) => `${l.name} ${t("order.qty", { qty: l.qty })}${l.lineCents === null ? "" : ` · ${formatLKR(l.lineCents)}`}`;

function Row({ change }: { change: LineChange }) {
  const tone = change.kind === "removed" ? "text-danger" : change.kind === "added" ? "text-success" : "";
  return (
    <li className={clsx("flex flex-col gap-0.5 py-2", tone)}>
      {change.kind === "removed" && <><span className="line-through">{text(change.was)}</span><span className="text-sm font-semibold">{t("order.removed")}</span></>}
      {change.kind === "added" && <><span>{text(change.now)}</span><span className="text-sm font-semibold">{t("order.added")}</span></>}
      {change.kind === "changed" && <><span className="text-sm text-muted line-through">{t("order.was")}: {text(change.was)}</span><span>{t("order.now")}: {text(change.now)}</span></>}
      {change.kind === "same" && <span>{text(change.now)}</span>}
    </li>
  );
}

/** Old against new, so the person sees exactly what the shop changed before accepting. */
export function RevisionCompare({ order }: { order: OrderDetail }) {
  const changes = diffLines(order.previousLines, order.lines);
  return (
    <Banner tone="warning">
      <p className="font-semibold">{t("order.revisionTitle")}</p>
      <p className="mb-1 text-sm">{t("order.revisionBody")}</p>
      <ul className="divide-y divide-line">{changes.map((c, i) => <Row key={i} change={c} />)}</ul>
    </Banner>
  );
}
