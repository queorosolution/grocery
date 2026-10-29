"use client";

import { useState, type FormEvent } from "react";
import { Banner, Button, Checkbox, Field, Input, Modal } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import { centsToRupeesText, formatLKR, parseRupees } from "@/lib/money";
import { usePatchProduct } from "./productsApi";
import type { ShopProduct } from "./types";

const wholeNumber = (text: string): number | null => (/^\d{1,9}$/.test(text.trim()) ? Number(text) : null);

/** Edit one product. The customer price is worked out by the server and shown, never typed. */
export function ProductEditDialog({ shopId, product: p, onClose }: { shopId: string; product: ShopProduct; onClose: () => void }) {
  const patch = usePatchProduct(shopId);
  const managed = p.managedBy === "CONNECTOR";
  const [price, setPrice] = useState(centsToRupeesText(p.shopPriceCents));
  const [mrp, setMrp] = useState(p.mrpCents === null ? "" : centsToRupeesText(p.mrpCents));
  const [stock, setStock] = useState(p.stockQty === null ? "" : String(p.stockQty));
  const [max, setMax] = useState(String(p.maxPerOrder));
  const [on, setOn] = useState(p.isAvailable);
  const [tried, setTried] = useState(false);

  const shopPriceCents = parseRupees(price);
  const mrpCents = mrp.trim() === "" ? null : parseRupees(mrp);
  const stockQty = stock.trim() === "" ? null : wholeNumber(stock);
  const maxPerOrder = wholeNumber(max);
  const bad = {
    price: shopPriceCents === null || shopPriceCents < 1, mrp: mrp.trim() !== "" && mrpCents === null,
    stock: stock.trim() !== "" && stockQty === null, max: maxPerOrder === null || maxPerOrder < 1,
  };
  const server = patch.error instanceof ApiError ? patch.error.fields : {};
  const shown = (name: keyof typeof bad, field: string, text: string) => (tried && bad[name] ? text : server[field] ? errorText(patch.error) : undefined);

  const save = (e: FormEvent) => {
    e.preventDefault();
    setTried(true);
    if (Object.values(bad).some(Boolean) || maxPerOrder === null) return;
    // A connector-managed product only takes the order limit; the others would be refused.
    const body = managed ? { maxPerOrder } : { shopPriceCents: shopPriceCents!, mrpCents, stockQty, isAvailable: on, maxPerOrder };
    patch.mutate({ id: p.id, patch: body }, { onSuccess: onClose });
  };

  return (
    <Modal open onClose={onClose} title={t("shopConsole.editTitle", { name: p.name })}
      footer={<Button type="submit" form="product-form" className="flex-1" loading={patch.isPending}>{t("shopConsole.saveProduct")}</Button>}>
      <form id="product-form" onSubmit={save} noValidate className="flex flex-col gap-3">
        {managed && <Banner tone="info">{t("shopConsole.managed")}</Banner>}
        <Field label={t("shopConsole.shopPrice")} error={shown("price", "shopPriceCents", t("shopConsole.badMoney"))}>
          {(f) => <Input {...f} inputMode="decimal" disabled={managed} value={price} onChange={(e) => setPrice(e.target.value)} />}
        </Field>
        <Field label={t("shopConsole.mrp")} error={shown("mrp", "mrpCents", t("shopConsole.badMoney"))}>
          {(f) => <Input {...f} inputMode="decimal" disabled={managed} value={mrp} onChange={(e) => setMrp(e.target.value)} />}
        </Field>
        <Field label={t("shopConsole.stock")} error={shown("stock", "stockQty", t("shopConsole.badNumber"))}>
          {(f) => <Input {...f} inputMode="numeric" disabled={managed} value={stock} onChange={(e) => setStock(e.target.value)} />}
        </Field>
        <Field label={t("shopConsole.maxPerOrder")} error={shown("max", "maxPerOrder", t("shopConsole.badNumber"))}>
          {(f) => <Input {...f} inputMode="numeric" value={max} onChange={(e) => setMax(e.target.value)} />}
        </Field>
        <Checkbox label={t("shopConsole.isAvailable")} checked={on} disabled={managed} onChange={setOn} />
        <p className="text-sm text-muted">{t("shopConsole.onlineIs", { price: formatLKR(p.onlinePriceCents) })}</p>
        {patch.error && Object.keys(server).length === 0 && <Banner tone="danger">{errorText(patch.error)}</Banner>}
      </form>
    </Modal>
  );
}
