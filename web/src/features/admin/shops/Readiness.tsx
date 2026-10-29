"use client";

import { Check, Circle } from "lucide-react";
import { Banner, Card, Switch } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import type { ReadinessItem, ShopDetail } from "../types";
import { useSetAccepting } from "./shopApi";

const ITEMS: readonly ReadinessItem[] = ["SHOP_ACTIVE", "HOURS", "PAYMENT_ACCOUNT", "FUTURE_SLOT"];
/** The tab that fixes each missing item. */
export const FIX_TAB: Record<ReadinessItem, string> = { SHOP_ACTIVE: "profile", HOURS: "hours", PAYMENT_ACCOUNT: "payment", FUTURE_SLOT: "slots" };

/** What is still missing before the shop can take orders, and the switch itself. The API checks again (SHOP_NOT_READY). */
export function Readiness({ detail, onGo }: { detail: ShopDetail; onGo: (tab: string) => void }) {
  const { shop, settings, readiness } = detail;
  const accepting = useSetAccepting(shop.id);
  // A courier shop needs no slots.
  const items = ITEMS.filter((i) => i !== "FUTURE_SLOT" || settings.deliveryType === "SLOT");
  const error = accepting.error;
  return (
    <Card className="mb-3 flex flex-col gap-2 p-3">
      <div className="flex items-center justify-between gap-3">
        <div>
          <div className="font-semibold">{t("admin.shop.readiness")}</div>
          <div className="text-sm text-muted">{readiness.ready ? t("admin.shop.ready") : t("admin.shop.notReady")}</div>
        </div>
        <Switch label={t("admin.shop.acceptingLabel")} checked={shop.acceptingOrders} disabled={accepting.isPending || (!readiness.ready && !shop.acceptingOrders)}
          onChange={(on) => accepting.mutate(on)} />
      </div>
      <ul className="flex flex-col">
        {items.map((i) => {
          const missing = readiness.missing.includes(i);
          return (
            <li key={i} className="flex min-h-9 items-center gap-2 text-sm">
              {missing ? <Circle size={16} aria-hidden="true" className="text-muted" /> : <Check size={16} aria-hidden="true" className="text-success" />}
              <span className={missing ? "" : "text-muted"}>{t(`admin.shop.missing.${i}`)}</span>
              {missing && <button type="button" onClick={() => onGo(FIX_TAB[i])} className="ml-auto min-h-9 font-semibold text-accent underline">{t("admin.shop.fix")}</button>}
            </li>
          );
        })}
      </ul>
      {error && <Banner tone="danger">{error instanceof ApiError && error.code === "SHOP_NOT_READY" ? t("admin.shop.notReady") : errorText(error)}</Banner>}
    </Card>
  );
}
