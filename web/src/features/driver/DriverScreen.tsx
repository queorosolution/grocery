"use client";

import { Button, EmptyState, Skeleton, StatTile } from "@/components/ui";
import { Page } from "@/features/shell/Page";
import { t } from "@/lib/i18n";
import { colomboDay, formatDay, formatTime } from "@/lib/time";
import { useDeliveries, useDriverStats } from "./api";
import { DeliveryCard } from "./DeliveryCard";

function Stats({ today }: { today: string }) {
  // Both ends count, so the week is today and the six days before it.
  const day = useDriverStats(today, today);
  const week = useDriverStats(colomboDay(-6), today);
  const tile = (label: string, q: typeof day, key: "delivered" | "failed") =>
    <StatTile label={label} value={q.data ? q.data[key] : "–"} />;
  return (
    <section aria-label={t("driver.week")} className="grid grid-cols-2 gap-2">
      {tile(`${t("driver.today")} · ${t("driver.delivered")}`, day, "delivered")}
      {tile(`${t("driver.today")} · ${t("driver.failed")}`, day, "failed")}
      {tile(`${t("driver.week")} · ${t("driver.delivered")}`, week, "delivered")}
      {tile(`${t("driver.week")} · ${t("driver.failed")}`, week, "failed")}
    </section>
  );
}

/** /console/driver: today's drop-offs grouped by slot, then the day's numbers. */
export function DriverScreen() {
  const today = colomboDay(0);
  const list = useDeliveries(today);
  return (
    <Page className="flex max-w-xl flex-col gap-4">
      <div>
        <h1 className="text-xl font-semibold md:text-2xl">{t("driver.title")}</h1>
        <p className="text-muted">{formatDay(`${today}T12:00:00+05:30`)}</p>
      </div>
      {list.isPending ? (
        <div aria-busy="true" className="flex flex-col gap-3">{[0, 1].map((i) => <Skeleton key={i} className="h-40" />)}</div>
      ) : list.isError ? (
        <EmptyState title={t("common.error")} action={<Button onClick={() => void list.refetch()}>{t("common.retry")}</Button>} />
      ) : list.data.length === 0 ? (
        <EmptyState title={t("driver.empty")} body={t("driver.emptyBody")} />
      ) : (
        list.data.map((g) => (
          <section key={g.slot?.startsAt ?? "courier"} className="flex flex-col gap-2">
            <h2 className="text-base font-semibold">{g.slot ? `${formatTime(g.slot.startsAt)} – ${formatTime(g.slot.endsAt)}` : t("driver.courier")}</h2>
            {g.orders.map((o) => <DeliveryCard key={o.id} delivery={o} />)}
          </section>
        ))
      )}
      <Stats today={today} />
    </Page>
  );
}
