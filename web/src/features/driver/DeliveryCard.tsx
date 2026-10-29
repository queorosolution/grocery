"use client";

import { MapPin, Phone } from "lucide-react";
import { useState } from "react";
import { Badge, Button, Card, statusTone } from "@/components/ui";
import { dialable, mapUrl } from "@/lib/contact";
import { t } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import { statusText } from "@/features/orders/statusText";
import { CodeDialog } from "./CodeDialog";
import { FailDialog } from "./FailDialog";
import type { Delivery } from "./types";

const linkClass = "inline-flex min-h-11 flex-1 items-center justify-center gap-2 rounded-input border border-line px-3 font-semibold text-accent";

/** One drop-off: who, where, how much, and the buttons for what the driver can do now. */
export function DeliveryCard({ delivery }: { delivery: Delivery }) {
  const { customer, address } = delivery;
  const [dialog, setDialog] = useState<"code" | "fail" | null>(null);
  // Kept here, not in the dialog, so closing the sheet does not forget the wrong tries.
  const [wrong, setWrong] = useState(0);
  const out = delivery.status === "OUT_FOR_DELIVERY";
  const pin = mapUrl(address.lat, address.lng);

  return (
    <Card className="flex flex-col gap-3 p-4">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="truncate text-base font-semibold">{customer.name}</div>
          <div className="text-sm text-muted">{delivery.code} · {t("driver.items", { count: delivery.itemCount })} · {formatLKR(delivery.totalCents)}</div>
        </div>
        <Badge tone={statusTone(delivery.status)}>{t(statusText[delivery.status])}</Badge>
      </div>
      <div>
        <div className="font-semibold">{address.label}</div>
        <div>{address.line1}{address.landmark ? `, ${address.landmark}` : ""}</div>
      </div>
      <div className="flex gap-2">
        <a href={`tel:${dialable(customer.phone)}`} aria-label={t("driver.call", { name: customer.name })} className={linkClass}>
          <Phone size={18} aria-hidden="true" />{customer.phone}
        </a>
        {pin && (
          <a href={pin} target="_blank" rel="noopener noreferrer" className={linkClass}>
            <MapPin size={18} aria-hidden="true" />{t("driver.map")}
          </a>
        )}
      </div>
      {out && (
        <div className="flex gap-2">
          <Button className="h-12 flex-1" onClick={() => setDialog("code")}>{t("driver.enterCode")}</Button>
          <Button variant="secondary" className="h-12 flex-1" onClick={() => setDialog("fail")}>{t("driver.cantDeliver")}</Button>
        </div>
      )}
      <CodeDialog delivery={delivery} wrong={wrong} onWrong={setWrong} open={dialog === "code"} onClose={() => setDialog(null)} />
      <FailDialog delivery={delivery} open={dialog === "fail"} onClose={() => setDialog(null)} />
    </Card>
  );
}
