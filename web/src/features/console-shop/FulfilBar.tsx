"use client";

import { useState } from "react";
import { Banner, Button, Card, Select } from "@/components/ui";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import { useAssignDriver, useDrivers, useSetStatus } from "./api";
import { RefundDialog } from "./RefundDialog";
import type { ShopOrder } from "./types";

// Each button moves the order one step. The server decides which of them exist (allowedActions)
// and refuses a step out of turn.
const STEPS = [
  ["START_PICKING", "PICKING", "shopConsole.startPicking"],
  ["MARK_PACKED", "PACKED", "shopConsole.markPacked"],
  ["SEND_OUT", "OUT_FOR_DELIVERY", "shopConsole.sendOut"],
  ["RETRY_DELIVERY", "OUT_FOR_DELIVERY", "shopConsole.retryDelivery"],
] as const;

export function FulfilBar({ shopId, order }: { shopId: string; order: ShopOrder }) {
  const can = (a: string) => order.allowedActions.includes(a);
  const status = useSetStatus(shopId, order.id);
  const assign = useAssignDriver(shopId, order.id);
  const drivers = useDrivers(shopId, can("ASSIGN_DRIVER"));
  const [driver, setDriver] = useState("");
  const [refund, setRefund] = useState<"FULL" | "PARTIAL" | null>(null);
  const steps = STEPS.filter(([action]) => can(action));
  const error = status.error ?? assign.error;
  if (steps.length === 0 && !can("ASSIGN_DRIVER") && !can("REQUEST_CANCEL") && !can("REQUEST_PARTIAL_REFUND")) return null;

  return (
    <Card className="flex flex-col gap-2 p-4">
      {can("ASSIGN_DRIVER") && (
        <div className="flex gap-2">
          <Select aria-label={t("shopConsole.driver")} className="flex-1" value={driver} onChange={(e) => setDriver(e.target.value)}>
            <option value="">{t("shopConsole.chooseDriver")}</option>
            {drivers.data?.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
          </Select>
          <Button variant="secondary" disabled={!driver} loading={assign.isPending}
            onClick={() => assign.mutate({ version: order.version, driverId: driver })}>{t("shopConsole.assign")}</Button>
        </div>
      )}
      {steps.map(([action, to, label]) => (
        <Button key={action} size="lg" loading={status.isPending && status.variables?.to === to}
          onClick={() => status.mutate({ version: order.version, to })}>{t(label)}</Button>
      ))}
      {can("REQUEST_PARTIAL_REFUND") && <Button variant="secondary" onClick={() => setRefund("PARTIAL")}>{t("shopConsole.refundPartial")}</Button>}
      {can("REQUEST_CANCEL") && <Button variant="ghost" onClick={() => setRefund("FULL")}>{t("shopConsole.refundFull")}</Button>}
      {error && <Banner tone="danger">{errorText(error)}</Banner>}
      {refund && <RefundDialog shopId={shopId} order={order} type={refund} open onClose={() => setRefund(null)} />}
    </Card>
  );
}
