"use client";

import Link from "next/link";
import { useState } from "react";
import { Volume2, VolumeX } from "lucide-react";
import { Badge, Button, EmptyState, Skeleton, TabPanel, Tabs } from "@/components/ui";
import { statusTone } from "@/components/ui/statusTone";
import { statusText } from "@/features/orders/statusText";
import { unlockChime } from "@/lib/chime";
import { t } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import { formatDateTime } from "@/lib/time";
import { useQueue } from "./api";
import { QUEUE_TABS } from "./queueTabs";
import { useNewBillChime } from "./useNewBillChime";

function Rows({ shopId, tab }: { shopId: string; tab: (typeof QUEUE_TABS)[number] }) {
  const q = useQueue(shopId, tab.statuses);
  const rows = q.data?.pages.flatMap((p) => p.data) ?? [];
  if (q.isPending) return <div className="flex flex-col gap-2" aria-busy="true">{[0, 1, 2].map((i) => <Skeleton key={i} className="h-20" />)}</div>;
  if (q.isError) return <EmptyState title={t("common.error")} action={<Button variant="secondary" onClick={() => void q.refetch()}>{t("common.retry")}</Button>} />;
  if (rows.length === 0) return <EmptyState title={t("shopConsole.queueEmpty")} />;
  return (
    <>
      <ul className="flex flex-col gap-2">
        {rows.map((o) => (
          <li key={o.id}>
            <Link href={`/console/shop/${shopId}/order/${o.id}`} className="flex items-center justify-between gap-3 rounded-card border border-line bg-bg p-3 md:hover:shadow-raise">
              <div className="min-w-0">
                <div className="truncate font-semibold">{o.code} · {o.customer.name}</div>
                <div className="truncate text-sm text-muted">
                  {t("order.listItems", { count: o.itemCount })} · {o.slot ? formatDateTime(o.slot.startsAt) : formatDateTime(o.createdAt)}
                </div>
              </div>
              <div className="flex shrink-0 flex-col items-end gap-1">
                <span className="font-semibold">{formatLKR(o.totalCents)}</span>
                <Badge tone={statusTone(o.status)}>{t(statusText[o.status])}</Badge>
              </div>
            </Link>
          </li>
        ))}
      </ul>
      {q.hasNextPage && <div className="mt-4 flex justify-center"><Button variant="secondary" loading={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>{t("shop.more")}</Button></div>}
    </>
  );
}

/** The shop's queue. New bills ring (after the sound is switched on with a tap, which browsers require). */
export function OrderQueue({ shopId }: { shopId: string }) {
  const [tab, setTab] = useState(QUEUE_TABS[0]!.id);
  const [sound, setSound] = useState(false);
  // The new-bill list is always watched, so a bill rings whichever tab is open.
  const fresh = useQueue(shopId, QUEUE_TABS[0]!.statuses);
  useNewBillChime(fresh.data?.pages.flatMap((p) => p.data.map((o) => o.id)), sound);
  const newCount = fresh.data?.pages[0]?.data.length ?? 0;
  const current = QUEUE_TABS.find((x) => x.id === tab) ?? QUEUE_TABS[0]!;

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-start justify-between gap-2">
        <Tabs className="min-w-0 flex-1" tabs={QUEUE_TABS.map((x) => ({ id: x.id, label: x.id === "new" && newCount > 0 ? `${t(x.label)} (${newCount})` : t(x.label) }))}
          value={tab} onChange={setTab} label={t("shopConsole.queue")} idPrefix="queue" />
        <Button variant="ghost" aria-pressed={sound} aria-label={t(sound ? "shopConsole.soundOn" : "shopConsole.soundOff")}
          onClick={() => setSound((on) => (on ? false : unlockChime()))}>
          {sound ? <Volume2 size={20} aria-hidden="true" /> : <VolumeX size={20} aria-hidden="true" />}
        </Button>
      </div>
      <TabPanel id={current.id} idPrefix="queue" active><Rows shopId={shopId} tab={current} /></TabPanel>
    </div>
  );
}
