"use client";

import { useState } from "react";
import { Badge, Button, Chips, DataTable, EmptyState, Input, type Column } from "@/components/ui";
import { useDebounced } from "@/lib/useDebounced";
import { t } from "@/lib/i18n";
import { AdminFrame } from "../AdminFrame";
import type { Category, Product } from "../types";
import { useCategories, useProducts } from "./api";
import { CategoryDialog } from "./CategoryDialog";
import { ProductDialog } from "./ProductDialog";

function Products({ categories }: { categories: Category[] }) {
  const [term, setTerm] = useState("");
  const q = useDebounced(term.trim(), 300);
  const products = useProducts(q);
  // `false` = closed, `null` = adding, a product = changing it.
  const [editing, setEditing] = useState<Product | null | false>(false);
  const rows = products.data?.pages.flatMap((p) => p.data) ?? [];
  const names = new Map(categories.map((c) => [c.id, c.name]));
  const columns: Column<Product>[] = [
    { key: "name", header: t("admin.shops.name"), cell: (p) => <span><span className="font-semibold">{p.name}</span><span className="block text-xs text-muted">{[p.brand, p.sizeLabel].filter(Boolean).join(" · ")}</span></span> },
    { key: "barcode", header: t("admin.connector.col.barcode"), cell: (p) => p.barcode ?? "–", hideOnMobile: true },
    { key: "category", header: t("admin.catalog.category"), cell: (p) => (p.categoryId ? names.get(p.categoryId) ?? "–" : "–"), hideOnMobile: true },
    { key: "status", header: t("admin.shops.status"), cell: (p) => <Badge tone={p.status === "ACTIVE" ? "success" : "neutral"}>{t(`admin.catalog.status_${p.status}`)}</Badge> },
  ];
  return (
    <div className="flex flex-col gap-3">
      <div className="flex gap-2">
        <Input aria-label={t("admin.catalog.search")} placeholder={t("admin.catalog.search")} value={term} onChange={(e) => setTerm(e.target.value)} />
        <Button className="shrink-0" onClick={() => setEditing(null)}>{t("admin.catalog.newProduct")}</Button>
      </div>
      {products.isError ? <EmptyState title={t("common.error")} /> : (
        <DataTable caption={t("admin.catalog.products")} columns={columns} rows={rows} rowKey={(p) => p.id} loading={products.isPending}
          emptyTitle={t("admin.catalog.noProducts")} onRowClick={(p) => setEditing(p)} />
      )}
      {products.hasNextPage && <Button variant="secondary" className="self-center" loading={products.isFetchingNextPage} onClick={() => void products.fetchNextPage()}>{t("admin.more")}</Button>}
      <ProductDialog key={editing ? editing.id : "new"} product={editing || null} categories={categories} open={editing !== false} onClose={() => setEditing(false)} />
    </div>
  );
}

function Categories({ categories }: { categories: Category[] }) {
  const [editing, setEditing] = useState<Category | null | false>(false);
  const tops = categories.filter((c) => c.parentId === null);
  return (
    <div className="flex flex-col gap-3">
      <Button className="self-start" onClick={() => setEditing(null)}>{t("admin.catalog.newCategory")}</Button>
      {categories.length === 0 ? <EmptyState title={t("admin.catalog.noCategories")} /> : (
        <ul className="divide-y divide-line rounded-card border border-line">
          {tops.map((c) => (
            <li key={c.id}>
              <button type="button" className="flex min-h-11 w-full items-center px-3 text-left font-semibold" onClick={() => setEditing(c)}>{c.name}</button>
              {categories.filter((k) => k.parentId === c.id).map((k) => (
                <button key={k.id} type="button" className="flex min-h-11 w-full items-center pl-8 pr-3 text-left" onClick={() => setEditing(k)}>{k.name}</button>
              ))}
            </li>
          ))}
        </ul>
      )}
      <CategoryDialog key={editing ? editing.id : "new"} category={editing || null} all={categories} open={editing !== false} onClose={() => setEditing(false)} />
    </div>
  );
}

export function CatalogScreen() {
  const [part, setPart] = useState<"products" | "categories">("products");
  const categories = useCategories();
  return (
    <AdminFrame title={t("admin.section.catalog")}>
      <div className="flex flex-col gap-3">
        <Chips label={t("admin.section.catalog")} value={part} onChange={(v) => setPart(v === "categories" ? "categories" : "products")}
          items={[{ id: "products", label: t("admin.catalog.products") }, { id: "categories", label: t("admin.catalog.categories") }]} />
        {part === "products" ? <Products categories={categories.data?.data ?? []} /> : <Categories categories={categories.data?.data ?? []} />}
      </div>
    </AdminFrame>
  );
}
