"use client";

import { useState } from "react";
import { Badge, Button, Chips, EmptyState, Select, Skeleton } from "@/components/ui";
import { t, type MessageKey } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import { AdminFrame } from "../AdminFrame";
import { useShops } from "../shops/shopApi";
import type { SyncIssue } from "../types";
import { useSyncIssues } from "./api";
import { ResolveDialog } from "./ResolveDialog";

const STATUSES: SyncIssue["status"][] = ["OPEN", "RESOLVED", "IGNORED"];

function Row({ issue, shop }: { issue: SyncIssue; shop: string }) {
  const [open, setOpen] = useState(false);
  const p = issue.payload;
  return (
    <li className="flex flex-wrap items-center gap-2 px-3 py-2">
      <div className="min-w-0 flex-1">
        <div className="truncate font-semibold">{p.name ?? issue.externalId}</div>
        <div className="truncate text-sm text-muted">{shop} · {p.priceCents !== null ? formatLKR(p.priceCents) : "–"}{p.currentPriceCents !== undefined ? ` (was ${formatLKR(p.currentPriceCents)})` : ""}</div>
      </div>
      <Badge tone={issue.kind === "UNMATCHED" ? "warning" : "info"}>{t(`admin.issues.kind.${issue.kind}` as MessageKey)}</Badge>
      {issue.status === "OPEN" && <Button variant="secondary" onClick={() => setOpen(true)}>{t("admin.issues.resolve")}</Button>}
      {open && <ResolveDialog issue={issue} open onClose={() => setOpen(false)} />}
    </li>
  );
}

/** What the sync could not decide alone: rows with no match and prices held back. Open ones first. */
export function SyncIssuesScreen() {
  const [status, setStatus] = useState<SyncIssue["status"]>("OPEN");
  const [shopId, setShopId] = useState("");
  const shops = useShops();
  const issues = useSyncIssues(status, shopId || null);
  const names = new Map(shops.data?.data.map((s) => [s.id, s.name]));
  return (
    <AdminFrame title={t("admin.section.issues")}>
      <div className="mb-3 flex flex-col gap-2 md:flex-row">
        <Chips label={t("admin.section.issues")} value={status} onChange={(v) => setStatus((v as SyncIssue["status"]) ?? "OPEN")} items={STATUSES.map((s) => ({ id: s, label: t(`admin.issues.status.${s}` as MessageKey) }))} />
        <Select aria-label={t("admin.issues.shop")} className="md:w-60" value={shopId} onChange={(e) => setShopId(e.target.value)}>
          <option value="">{t("admin.issues.allShops")}</option>
          {shops.data?.data.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
        </Select>
      </div>
      {issues.isPending ? <Skeleton className="h-32" /> : issues.isError ? <EmptyState title={t("common.error")} /> : issues.data.data.length === 0 ? <EmptyState title={t("admin.issues.empty")} /> : (
        <ul className="divide-y divide-line rounded-card border border-line">
          {issues.data.data.map((i) => <Row key={i.id} issue={i} shop={names.get(i.shopId) ?? "–"} />)}
        </ul>
      )}
    </AdminFrame>
  );
}
