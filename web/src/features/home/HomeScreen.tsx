"use client";

import { Store } from "lucide-react";
import { EmptyState } from "@/components/ui";
import { Page } from "@/features/shell/Page";
import { t } from "@/lib/i18n";

export function HomeScreen() {
  return (
    <Page>
      <h1 className="text-xl font-semibold md:text-2xl">{t("home.title")}</h1>
      <EmptyState icon={<Store size={32} />} title={t("home.emptyTitle")} body={t("home.emptyBody")} />
    </Page>
  );
}
