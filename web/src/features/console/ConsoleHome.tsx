"use client";

import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { Card } from "@/components/ui";
import { useMe } from "@/features/auth/api";
import { consoleLinks } from "@/features/auth/roles";
import { Page } from "@/features/shell/Page";
import { t } from "@/lib/i18n";

/** The console landing page: one card for each place this person works (their role decides). */
export function ConsoleHome() {
  const { me } = useMe();
  const links = me ? consoleLinks(me) : [];
  return (
    <Page className="max-w-xl">
      <h1 className="mb-1 text-xl font-semibold md:text-2xl">{t("console.title")}</h1>
      <p className="mb-4 text-muted">{t("console.body")}</p>
      <ul className="flex flex-col gap-2">
        {links.map((l) => (
          <li key={l.href}>
            <Card interactive>
              <Link href={l.href} className="flex min-h-16 items-center justify-between gap-3 px-4 py-3">
                <span className="min-w-0">
                  <span className="block truncate font-semibold">{l.title}</span>
                  <span className="block text-sm text-muted">{l.hint}</span>
                </span>
                <ChevronRight size={20} aria-hidden="true" className="shrink-0 text-muted" />
              </Link>
            </Card>
          </li>
        ))}
      </ul>
    </Page>
  );
}
