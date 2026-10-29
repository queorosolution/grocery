"use client";

import { KeyRound } from "lucide-react";
import { useState } from "react";
import { Badge, Button, Card, EmptyState, Skeleton } from "@/components/ui";
import { t } from "@/lib/i18n";
import type { PaymentAccount, ShopDetail } from "../types";
import { AccountDialog } from "./AccountDialog";
import { usePaymentAccounts } from "./paymentApi";

/** The shop's payment accounts. A secret is never listed: only whether one is saved. */
export function PaymentTab({ detail }: { detail: ShopDetail }) {
  const id = detail.shop.id;
  const accounts = usePaymentAccounts();
  // `false` = closed, `null` = adding, an account = changing it.
  const [editing, setEditing] = useState<PaymentAccount | null | false>(false);
  if (accounts.isPending) return <Skeleton className="h-32" />;
  if (accounts.isError) return <EmptyState title={t("common.error")} />;
  const mine = accounts.data.data.filter((a) => a.shopId === id);
  return (
    <div className="flex flex-col gap-3">
      <Button className="self-start" onClick={() => setEditing(null)}>{t("admin.account.add")}</Button>
      {mine.length === 0 && <EmptyState title={t("admin.account.none")} body={t("admin.account.noneBody")} />}
      <ul className="flex flex-col gap-2">
        {mine.map((a) => (
          <li key={a.id}>
            <Card className="flex flex-wrap items-center gap-2 p-3">
              <div className="min-w-0 flex-1">
                <div className="font-semibold">{t(`admin.payment.provider_${a.provider}`)}{a.provider === "PAYHERE" ? ` · ${t(`admin.account.mode_${a.mode}`)}` : ""}</div>
                <div className="truncate text-sm text-muted">{a.provider === "PAYHERE" ? a.merchantId : a.bank ? `${a.bank.bankName} · ${a.bank.accountNumber}` : ""}</div>
              </div>
              {a.provider === "PAYHERE" && a.hasSecret && <span className="inline-flex items-center gap-1 text-sm text-muted"><KeyRound size={14} aria-hidden="true" />{t("admin.account.secretSaved")}</span>}
              <Badge tone={a.status === "ACTIVE" ? "success" : "neutral"}>{t(`admin.account.status_${a.status}`)}</Badge>
              <Button variant="secondary" onClick={() => setEditing(a)}>{t("admin.account.edit")}</Button>
            </Card>
          </li>
        ))}
      </ul>
      {/* A new key gives the dialog fresh boxes for each account, so one account's text never lands in another. */}
      <AccountDialog key={editing ? editing.id : "new"} shopId={id} account={editing || null} open={editing !== false} onClose={() => setEditing(false)} />
    </div>
  );
}
