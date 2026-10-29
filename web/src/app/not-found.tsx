"use client";

import Link from "next/link";
import { EmptyState, Skeleton } from "@/components/ui";
import { ShopScreen } from "@/features/shop/ShopScreen";
import { Header } from "@/features/shell/Header";
import { Page } from "@/features/shell/Page";
import { t } from "@/lib/i18n";
import { matchRoute, useClientPathname } from "@/lib/useRouteId";

// Cloudflare serves this page (404.html) for any path with no file. For /s/<slug>
// of a shop that is newer than the last build, show the real shop screen: the
// data comes from the API, so the shop works at once (docs/08 section 11.1).
export default function NotFound() {
  const pathname = useClientPathname();
  // Until the page has mounted we do not know the URL, so show neither message.
  const view = pathname === null ? "wait" : matchRoute("/s/:slug", pathname) ? "shop" : "missing";
  return (
    <>
      <Header />
      {view === "wait" && (
        <Page aria-busy="true">
          <Skeleton className="h-7 w-40" />
        </Page>
      )}
      {view === "shop" && <ShopScreen />}
      {view === "missing" && (
        <Page>
          <EmptyState
            title={t("notFound.title")}
            body={t("notFound.body")}
            action={<Link href="/" className="font-semibold text-accent underline">{t("notFound.home")}</Link>}
          />
        </Page>
      )}
    </>
  );
}
