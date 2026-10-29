"use client";

import { useState } from "react";
import { Badge, Button, Checkbox, DataTable, Input, type Column } from "@/components/ui";
import { shopPermissions } from "@/features/auth/roles";
import { useMe } from "@/features/auth/api";
import { t } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import { useRouteId } from "@/lib/useRouteId";
import { useDebounced } from "@/lib/useDebounced";
import { ProductEditDialog } from "./ProductEditDialog";
import { useShopProducts } from "./productsApi";
import { ShopFrame } from "./ShopFrame";
import type { ShopProduct } from "./types";

function Table({ shopId }: { shopId: string }) {
  const { me } = useMe();
  const canEdit = shopPermissions(me, shopId).has("product.edit");
  const [text, setText] = useState("");
  const q = useDebounced(text, 300);
  const [onlyOn, setOnlyOn] = useState(false);
  const [editing, setEditing] = useState<ShopProduct | null>(null);
  const list = useShopProducts(shopId, q, onlyOn ? true : null);
  const rows = list.data?.pages.flatMap((p) => p.data) ?? [];

  const columns: Column<ShopProduct>[] = [
    { key: "name", header: t("shopConsole.colName"), cell: (p) => <><div>{p.name}</div><div className="text-muted">{[p.brand, p.sizeLabel].filter(Boolean).join(" · ")}</div></> },
    { key: "price", header: t("shopConsole.colPrice"), align: "right", cell: (p) => formatLKR(p.shopPriceCents) },
    { key: "online", header: t("shopConsole.colOnline"), align: "right", hideOnMobile: true, cell: (p) => formatLKR(p.onlinePriceCents) },
    { key: "stock", header: t("shopConsole.colStock"), align: "right", hideOnMobile: true, cell: (p) => p.stockQty ?? "–" },
    { key: "status", header: t("shopConsole.colStatus"), cell: (p) => <Badge tone={p.isAvailable ? "success" : "neutral"}>{p.isAvailable ? t("shopConsole.available") : t("shopConsole.hidden")}</Badge> },
    ...(canEdit ? [{ key: "edit", header: "", align: "right" as const, cell: (p: ShopProduct) => <Button variant="ghost" aria-label={`${t("shopConsole.edit")} ${p.name}`} onClick={() => setEditing(p)}>{t("shopConsole.edit")}</Button> }] : []),
  ];

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-3">
        <Input className="max-w-xs flex-1" aria-label={t("shopConsole.searchProducts")} placeholder={t("shopConsole.searchProducts")} value={text} onChange={(e) => setText(e.target.value)} />
        <Checkbox label={t("shopConsole.onlyAvailable")} checked={onlyOn} onChange={setOnlyOn} />
      </div>
      <DataTable caption={t("shopConsole.products")} columns={columns} rows={rows} rowKey={(p) => p.id} loading={list.isPending} emptyTitle={t("shopConsole.productsEmpty")} />
      {list.hasNextPage && <div className="flex justify-center"><Button variant="secondary" loading={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>{t("shop.more")}</Button></div>}
      {editing && <ProductEditDialog shopId={shopId} product={editing} onClose={() => setEditing(null)} />}
    </div>
  );
}

export function ProductsScreen() {
  const shopId = useRouteId("/console/shop/:shopId/products", "shopId");
  return <ShopFrame shopId={shopId}>{(id) => <Table shopId={id} />}</ShopFrame>;
}
