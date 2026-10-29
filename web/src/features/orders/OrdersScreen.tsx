"use client";

import Link from "next/link";
import { Receipt } from "lucide-react";
import { Badge, Button, EmptyState, Skeleton } from "@/components/ui";
import { statusTone } from "@/components/ui/statusTone";
import { RequireUser } from "@/features/auth/guards";
import { Page } from "@/features/shell/Page";
import { t } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import { formatDay } from "@/lib/time";
import { useOrders } from "./api";
import { statusText } from "./statusText";

function List() {
  const q = useOrders();
  const rows = q.data?.pages.flatMap((p) => p.data) ?? [];
  if (q.isPending) return <div className="flex flex-col gap-2" aria-busy="true">{[0, 1, 2].map((i) => <Skeleton key={i} className="h-20" />)}</div>;
  if (q.isError) return <EmptyState title={t("common.error")} action={<Button variant="secondary" onClick={() => void q.refetch()}>{t("common.retry")}</Button>} />;
  if (rows.length === 0) return <EmptyState icon={<Receipt size={32} />} title={t("order.listEmptyTitle")} body={t("order.listEmptyBody")} />;
  return (
    <>
      <ul className="flex flex-col gap-2">
        {rows.map((o) => (
          <li key={o.id}>
            <Link href={`/order/${o.id}`} className="flex items-center justify-between gap-3 rounded-card border border-line bg-bg p-3 md:hover:shadow-raise">
              <div className="min-w-0">
                <div className="truncate font-semibold">{o.shop.name}</div>
                <div className="text-sm text-muted">{o.code} · {formatDay(o.createdAt)} · {t("order.listItems", { count: o.itemCount })}</div>
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

export function OrdersScreen() {
  return (
    <Page>
      <h1 className="mb-3 text-xl font-semibold">{t("order.listTitle")}</h1>
      <RequireUser><List /></RequireUser>
    </Page>
  );
}
