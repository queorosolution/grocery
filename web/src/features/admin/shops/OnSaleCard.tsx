"use client";

import { useState } from "react";
import { Button, Card, Checkbox, Field, Input } from "@/components/ui";
import { t } from "@/lib/i18n";
import { parseRupees } from "@/lib/money";
import { useDebounced } from "@/lib/useDebounced";
import { useAddShopProduct } from "../catalog/api";
import { MoneyField, NumberField } from "../fields";
import { fieldText, parseInt10, SaveNotice } from "../form";
import { useProductSearch } from "../productsApi";
import type { MasterProduct } from "../types";

/** Puts a catalog product on sale in this shop by hand. Prices later change through the shop console. */
export function OnSaleCard({ shopId }: { shopId: string }) {
  const add = useAddShopProduct(shopId);
  const [term, setTerm] = useState("");
  const found = useProductSearch(useDebounced(term, 300));
  const [picked, setPicked] = useState<MasterProduct | null>(null);
  const [price, setPrice] = useState("");
  const [mrp, setMrp] = useState("");
  const [stock, setStock] = useState("");
  const [max, setMax] = useState("20");
  const [available, setAvailable] = useState(true);
  const priceCents = parseRupees(price);
  const mrpCents = mrp.trim() === "" ? undefined : parseRupees(mrp);
  const stockQty = stock.trim() === "" ? undefined : parseInt10(stock);
  const maxPerOrder = parseInt10(max);
  const valid = !!picked && priceCents !== null && priceCents > 0 && mrpCents !== null && stockQty !== null && maxPerOrder !== null && maxPerOrder >= 1;

  const submit = () => {
    if (!valid) return;
    add.mutate({ productId: picked.id, shopPriceCents: priceCents, ...(mrpCents !== undefined ? { mrpCents } : {}), ...(stockQty !== undefined ? { stockQty } : {}), isAvailable: available, maxPerOrder },
      { onSuccess: () => { setPicked(null); setTerm(""); setPrice(""); setMrp(""); setStock(""); } });
  };

  return (
    <Card className="flex flex-col gap-3 p-4">
      <h2 className="text-lg font-semibold">{t("admin.onsale.title")}</h2>
      {picked ? (
        <div className="flex items-center justify-between gap-2">
          <span className="font-semibold">{picked.name}{picked.sizeLabel ? ` · ${picked.sizeLabel}` : ""}</span>
          <Button variant="secondary" onClick={() => setPicked(null)}>{t("admin.onsale.change")}</Button>
        </div>
      ) : (
        <>
          <Field label={t("admin.onsale.search")} error={fieldText(add.error, "productId")}>{(p) => <Input {...p} value={term} onChange={(e) => setTerm(e.target.value)} />}</Field>
          <ul className="flex flex-col gap-1">
            {found.data?.data.slice(0, 6).map((m) => (
              <li key={m.id}><Button variant="secondary" className="w-full justify-start text-left" onClick={() => setPicked(m)}>{m.name}{m.sizeLabel ? ` · ${m.sizeLabel}` : ""}{m.brand ? ` · ${m.brand}` : ""}</Button></li>
            ))}
          </ul>
        </>
      )}
      {picked && (
        <form className="grid gap-3 sm:grid-cols-2" onSubmit={(e) => { e.preventDefault(); submit(); }}>
          <MoneyField label={t("admin.onsale.price")} value={price} onChange={setPrice} error={fieldText(add.error, "shopPriceCents")} />
          <MoneyField label={t("admin.onsale.mrp")} hint={t("admin.fees.blank")} value={mrp} onChange={setMrp} error={fieldText(add.error, "mrpCents")} />
          <NumberField label={t("admin.onsale.stock")} hint={t("admin.fees.blank")} value={stock} onChange={setStock} />
          <NumberField label={t("admin.onsale.max")} value={max} onChange={setMax} />
          <Checkbox label={t("admin.onsale.available")} checked={available} onChange={setAvailable} className="sm:col-span-2" />
          <div className="flex flex-col gap-2 sm:col-span-2">
            <SaveNotice error={add.error} saved={add.isSuccess} />
            <Button type="submit" loading={add.isPending} disabled={!valid} className="self-start">{t("admin.onsale.add")}</Button>
          </div>
        </form>
      )}
    </Card>
  );
}
