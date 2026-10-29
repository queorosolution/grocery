"use client";

import clsx from "clsx";
import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";
import { EmptyState, Skeleton } from "@/components/ui";
import { useMe } from "@/features/auth/api";
import { shopPermissions } from "@/features/auth/roles";
import { Page } from "@/features/shell/Page";
import type { Permission } from "@/lib/enums.gen";
import { t, type MessageKey } from "@/lib/i18n";

const TABS: { path: string; label: MessageKey; needs: Permission }[] = [
  { path: "orders", label: "shopConsole.orders", needs: "order.review" },
  { path: "products", label: "shopConsole.products", needs: "order.review" },
  { path: "dashboard", label: "shopConsole.dashboard", needs: "dashboard.shop_ops" },
  { path: "payouts", label: "shopConsole.payouts", needs: "dashboard.shop_full" },
  { path: "team", label: "shopConsole.team", needs: "team.manage" },
];

/**
 * Around every shop console screen: the shop's name, the tabs this person may use, and a plain
 * message for someone with no place here. The API refuses them anyway (404 for another shop).
 */
export function ShopFrame({ shopId, children }: { shopId: string | null; children: (shopId: string) => ReactNode }) {
  const { me } = useMe();
  const pathname = usePathname();
  if (shopId === null) return <Page aria-busy="true"><Skeleton className="h-8 w-48" /></Page>;
  const perms = shopPermissions(me, shopId);
  if (!perms.has("order.review")) {
    return <Page><EmptyState title={t("guard.deniedTitle")} body={t("guard.deniedBody")} /></Page>;
  }
  const name = me?.roles.find((r) => r.shopId === shopId)?.shopName;
  return (
    <Page>
      <h1 className="mb-2 truncate text-xl font-semibold md:text-2xl">{name ?? t("console.shop")}</h1>
      <nav aria-label={t("shopConsole.nav")} className="-mx-4 mb-4 flex overflow-x-auto border-b border-line px-4 [scrollbar-width:none]">
        {TABS.filter((tab) => perms.has(tab.needs)).map((tab) => {
          const href = `/console/shop/${shopId}/${tab.path}`;
          const here = pathname.replace(/\/$/, "").startsWith(href) || (tab.path === "orders" && pathname.includes(`/console/shop/${shopId}/order/`));
          return (
            <Link key={tab.path} href={href} aria-current={here ? "page" : undefined}
              className={clsx("-mb-px flex h-11 shrink-0 items-center border-b-2 px-3 text-base", here ? "border-ink font-semibold" : "border-transparent text-muted hover:text-ink")}>
              {t(tab.label)}
            </Link>
          );
        })}
      </nav>
      {children(shopId)}
    </Page>
  );
}
