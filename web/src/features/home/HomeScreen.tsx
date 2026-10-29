"use client";

import { useRouter } from "next/navigation";
import { Store } from "lucide-react";
import { EmptyState, SearchBar2Way, Skeleton } from "@/components/ui";
import { Page } from "@/features/shell/Page";
import { t } from "@/lib/i18n";
import { useHome, usePin } from "./api";
import { DealTile } from "./DealTile";
import { NearMe } from "./NearMe";
import { ShopTile } from "./ShopTile";

export function HomeScreen() {
  const router = useRouter();
  const home = useHome();
  const pin = usePin();
  const shops = home.data?.shops ?? [];

  return (
    <Page className="flex flex-col gap-6">
      <SearchBar2Way
        shops={shops}
        onSearch={(q, shop) => router.push(`/search?q=${encodeURIComponent(q)}${shop ? `&shop=${encodeURIComponent(shop)}` : ""}`)}
      />

      <section aria-labelledby="near">
        <div className="mb-2 flex items-center justify-between gap-2">
          <h1 id="near" className="text-xl font-semibold">{pin ? t("home.title") : t("home.allShops")}</h1>
          <NearMe />
        </div>
        {home.isError ? (
          <EmptyState title={t("common.error")} />
        ) : home.isPending ? (
          <div className="grid gap-3 md:grid-cols-2" aria-busy="true">
            {[0, 1, 2].map((i) => <Skeleton key={i} className="h-20" />)}
          </div>
        ) : shops.length === 0 ? (
          <EmptyState icon={<Store size={32} />} title={pin ? t("home.noneNear") : t("home.emptyTitle")}
            body={pin ? t("home.noneNearBody") : t("home.emptyBody")} />
        ) : (
          <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
            {shops.map((s) => <ShopTile key={s.id} shop={s} />)}
          </div>
        )}
      </section>

      {(home.data?.deals.length ?? 0) > 0 && (
        <section aria-labelledby="deals">
          <h2 id="deals" className="mb-2 text-lg font-semibold">{t("home.deals")}</h2>
          <div className="-mx-4 flex gap-3 overflow-x-auto px-4 pb-1 [scrollbar-width:none]">
            {home.data?.deals.map((d) => <DealTile key={`${d.shopSlug}-${d.shopProductId}`} deal={d} />)}
          </div>
        </section>
      )}
    </Page>
  );
}
