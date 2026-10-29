"use client";

import { useState } from "react";
import { Button, Card, Field, Input, Skeleton } from "@/components/ui";
import { t } from "@/lib/i18n";
import { useCategories, useMarkups, useSaveMarkups } from "../catalog/api";
import { parseInt10, SaveNotice } from "../form";
import type { Category, Markup } from "../types";

function Form({ shopId, categories, initial }: { shopId: string; categories: Category[]; initial: Markup[] }) {
  const save = useSaveMarkups(shopId);
  const [values, setValues] = useState<Record<string, string>>(() => Object.fromEntries(initial.map((m) => [m.categoryId, String(m.markupBp)])));
  const parsed = categories.map((c) => ({ id: c.id, text: (values[c.id] ?? "").trim() }));
  const valid = parsed.every((p) => p.text === "" || (parseInt10(p.text) ?? 9999) <= 2000);
  const submit = () => save.mutate(parsed.filter((p) => p.text !== "").map((p) => ({ categoryId: p.id, markupBp: parseInt10(p.text)! })));
  return (
    <form className="flex flex-col gap-3" onSubmit={(e) => { e.preventDefault(); if (valid) submit(); }}>
      <div className="grid gap-2 sm:grid-cols-2">
        {categories.map((c) => (
          <Field key={c.id} label={c.parentId ? `– ${c.name}` : c.name}>
            {(p) => <Input {...p} inputMode="numeric" placeholder="0" value={values[c.id] ?? ""} onChange={(e) => setValues((v) => ({ ...v, [c.id]: e.target.value }))} aria-invalid={(values[c.id] ?? "") !== "" && (parseInt10(values[c.id]!) ?? 9999) > 2000 ? true : undefined} />}
          </Field>
        ))}
      </div>
      <SaveNotice error={save.error} saved={save.isSuccess} />
      <Button type="submit" loading={save.isPending} disabled={!valid} className="self-start">{t("admin.markups.save")}</Button>
    </form>
  );
}

/** Extra price by category, in basis points (100 = 1%). Saving prices every product of the shop again. */
export function MarkupsCard({ shopId }: { shopId: string }) {
  const categories = useCategories();
  const markups = useMarkups(shopId);
  return (
    <Card className="flex flex-col gap-2 p-4">
      <h2 className="text-lg font-semibold">{t("admin.markups.title")}</h2>
      <p className="text-sm text-muted">{t("admin.markups.body")}</p>
      {categories.isPending || markups.isPending ? <Skeleton className="h-24" /> : (
        <Form key={markups.dataUpdatedAt} shopId={shopId} categories={categories.data?.data ?? []} initial={markups.data?.data ?? []} />
      )}
    </Card>
  );
}
