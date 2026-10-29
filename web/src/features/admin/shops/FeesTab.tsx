"use client";

import { useState } from "react";
import { Chips, EmptyState, Skeleton } from "@/components/ui";
import { t } from "@/lib/i18n";
import type { FeeRule, ShopDetail } from "../types";
import { useFeeRules, useSaveRule } from "./feesApi";
import { FeePreview } from "./FeePreview";
import { RuleForm } from "./RuleForm";

const EMPTY: FeeRule = {
  baseCents: 0, perKmCents: 0, freeKm: 0, perKgCents: 0, freeKg: 0, freeAboveCents: null, minCents: 0, maxCents: null, maxKm: 10, roadFactor: 1.3,
};

/** This shop's own delivery fee rule, or the platform default that every shop without one uses. */
export function FeesTab({ detail }: { detail: ShopDetail }) {
  const id = detail.shop.id;
  const rules = useFeeRules();
  const [target, setTarget] = useState<"shop" | "default">("shop");
  const save = useSaveRule(target === "shop" ? id : null);
  if (rules.isPending) return <Skeleton className="h-40" />;
  if (rules.isError) return <EmptyState title={t("common.error")} />;
  const own = rules.data.data.find((r) => r.shopId === id);
  const platform = rules.data.data.find((r) => r.shopId === null);
  const shown = target === "shop" ? (own?.rule ?? platform?.rule ?? EMPTY) : (platform?.rule ?? EMPTY);
  return (
    <div className="flex flex-col gap-4">
      <Chips label={t("admin.shop.tab.fees")} value={target} onChange={(v) => { save.reset(); setTarget(v === "default" ? "default" : "shop"); }}
        items={[{ id: "shop", label: t("admin.fees.thisShop") }, { id: "default", label: t("admin.fees.platformDefault") }]} />
      <p className="text-sm text-muted">{target === "shop" ? (own ? t("admin.fees.ownRule") : t("admin.fees.usesDefault")) : t("admin.fees.defaultNote")}</p>
      {/* A new key remounts the form with the other rule's numbers. */}
      <RuleForm key={`${target}-${own?.updatedAt ?? ""}-${platform?.updatedAt ?? ""}`} initial={shown} pending={save.isPending} error={save.error} saved={save.isSuccess}
        submitLabel={t(target === "shop" ? "admin.fees.saveShop" : "admin.fees.saveDefault")} onSave={(rule) => save.mutate(rule)} />
      <FeePreview shopId={id} />
    </div>
  );
}
