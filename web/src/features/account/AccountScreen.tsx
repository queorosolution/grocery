"use client";

import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { Card } from "@/components/ui";
import { useMe } from "@/features/auth/api";
import { RequireUser } from "@/features/auth/guards";
import { Page } from "@/features/shell/Page";
import { t } from "@/lib/i18n";
import { ProfileCard } from "./ProfileCard";
import { SignOutCard } from "./SignOutCard";

function Body() {
  const { me } = useMe();
  if (!me) return null;
  return (
    <div className="flex flex-col gap-4">
      <ProfileCard me={me} />
      <Card interactive>
        <Link href="/account/addresses" className="flex min-h-14 items-center justify-between px-4">
          <span className="font-semibold">{t("account.addresses")}</span>
          <ChevronRight size={20} aria-hidden="true" className="text-muted" />
        </Link>
      </Card>
      <SignOutCard />
    </div>
  );
}

export function AccountScreen() {
  return (
    <Page className="max-w-xl">
      <h1 className="mb-4 text-xl font-semibold md:text-2xl">{t("account.title")}</h1>
      <RequireUser>
        <Body />
      </RequireUser>
    </Page>
  );
}
