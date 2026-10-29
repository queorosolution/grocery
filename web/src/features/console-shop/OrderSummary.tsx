import { Phone } from "lucide-react";
import { Card } from "@/components/ui";
import { dialable, mapUrl } from "@/lib/contact";
import { t } from "@/lib/i18n";
import { formatSlot } from "@/lib/time";
import type { ShopOrder } from "./types";

/** Who the order is for, where it goes and when. The full phone shows only while picking and delivering. */
export function OrderSummary({ order }: { order: ShopOrder }) {
  const { customer, address } = order;
  const phone = customer?.phone ? dialable(customer.phone) : null;
  const pin = mapUrl(address.lat, address.lng);
  return (
    <Card className="flex flex-col gap-3 p-4 text-sm">
      <div>
        <div className="text-muted">{t("shopConsole.customer")}</div>
        <div className="text-base font-semibold">{customer?.name}</div>
        {phone ? (
          <a href={`tel:${phone}`} className="inline-flex min-h-11 items-center gap-1 font-semibold text-accent underline"><Phone size={16} aria-hidden="true" />{customer?.phone}</a>
        ) : (
          <div className="text-muted">{customer?.phoneMasked}</div>
        )}
      </div>
      <div>
        <div className="text-muted">{t("order.deliverTo")}</div>
        <div>{address.label}: {address.line1}{address.landmark ? `, ${address.landmark}` : ""}</div>
        {pin && <a href={pin} target="_blank" rel="noopener noreferrer" className="inline-flex min-h-11 items-center font-semibold text-accent underline">{t("shopConsole.openMap")}</a>}
      </div>
      {order.slot && <div><div className="text-muted">{t("order.slot")}</div><div>{formatSlot(order.slot.startsAt, order.slot.endsAt)}</div></div>}
    </Card>
  );
}
