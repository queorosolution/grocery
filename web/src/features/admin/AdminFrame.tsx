"use client";

import clsx from "clsx";
import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";
import { Page } from "@/features/shell/Page";
import { t } from "@/lib/i18n";
import { SECTIONS } from "./sections";

const ROOT = "/console/admin";

/** Around every admin screen: a title and the menu of admin pages. The sign-in and TOTP checks sit above, in the layout. */
export function AdminFrame({ title, children }: { title: string; children: ReactNode }) {
  const pathname = (usePathname() ?? "").replace(/\/$/, "");
  return (
    <Page>
      <nav aria-label={t("admin.nav")} className="-mx-4 mb-3 flex overflow-x-auto border-b border-line px-4 [scrollbar-width:none]">
        {SECTIONS.map((s) => {
          const href = s.path ? `${ROOT}/${s.path}` : ROOT;
          // "shops" also covers /console/admin/shop/<id>.
          const here = s.path === "" ? pathname === ROOT : pathname.startsWith(href) || (s.path === "shops" && pathname.startsWith(`${ROOT}/shop/`));
          return (
            <Link key={s.path} href={href} aria-current={here ? "page" : undefined}
              className={clsx("-mb-px flex h-11 shrink-0 items-center border-b-2 px-3 text-base", here ? "border-ink font-semibold" : "border-transparent text-muted hover:text-ink")}>
              {t(s.label)}
            </Link>
          );
        })}
      </nav>
      <h1 className="mb-3 truncate text-xl font-semibold md:text-2xl">{title}</h1>
      {children}
    </Page>
  );
}
