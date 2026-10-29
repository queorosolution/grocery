import { Card } from "@/components/ui";
import { t } from "@/lib/i18n";

/** The code the driver asks for at the door. Only the customer gets one, only while out for delivery. */
export function DeliveryCodeCard({ code }: { code: string }) {
  return (
    <Card className="flex flex-col items-center gap-1 p-4 text-center">
      <p className="text-sm text-muted">{t("order.yourCode")}</p>
      <p className="text-2xl font-semibold tracking-[0.3em]" aria-label={code.split("").join(" ")}>{code}</p>
      <p className="text-sm text-muted">{t("order.codeBody")}</p>
    </Card>
  );
}
