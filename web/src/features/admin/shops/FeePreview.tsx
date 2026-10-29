"use client";

import { useState } from "react";
import { Banner, Button, Card } from "@/components/ui";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import { formatLKR, parseRupees } from "@/lib/money";
import { DecimalField, MoneyField } from "../fields";
import { parseDec2 } from "../form";
import { usePreviewFee } from "./feesApi";

/** "What would a customer pay?" for a distance, a weight and a bill. Nothing is saved. */
export function FeePreview({ shopId }: { shopId: string }) {
  const preview = usePreviewFee();
  const [km, setKm] = useState("3");
  const [kg, setKg] = useState("5");
  const [items, setItems] = useState("2000");
  const distanceKm = parseDec2(km);
  const weightKg = parseDec2(kg);
  const itemsCents = parseRupees(items);
  const ready = distanceKm !== null && weightKg !== null && itemsCents !== null;
  return (
    <Card className="flex flex-col gap-3 p-4">
      <h2 className="text-lg font-semibold">{t("admin.fees.previewTitle")}</h2>
      <div className="grid gap-3 sm:grid-cols-3">
        <DecimalField label={t("admin.fees.distance")} value={km} onChange={setKm} />
        <DecimalField label={t("admin.fees.weight")} value={kg} onChange={setKg} />
        <MoneyField label={t("admin.fees.items")} value={items} onChange={setItems} />
      </div>
      <Button variant="secondary" className="self-start" loading={preview.isPending} disabled={!ready}
        onClick={() => ready && preview.mutate({ shopId, distanceKm, weightKg, itemsCents })}>{t("admin.fees.previewRun")}</Button>
      {preview.data && <p role="status" className="font-semibold">{formatLKR(preview.data.feeCents)} <span className="font-normal text-muted">({t(preview.data.rule === "SHOP" ? "admin.fees.usedShop" : "admin.fees.usedDefault")})</span></p>}
      {preview.error && <Banner tone="danger">{errorText(preview.error)}</Banner>}
    </Card>
  );
}
