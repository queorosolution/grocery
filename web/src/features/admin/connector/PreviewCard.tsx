"use client";

import { Banner, Badge, Button, Card, DataTable, type Column, type Tone } from "@/components/ui";
import { errorText } from "@/lib/errorText";
import { t, type MessageKey } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import type { Connector, PreviewRow } from "../types";
import { usePreview } from "./connectorApi";

const TONE: Record<PreviewRow["action"], Tone> = { create: "success", update: "info", unmatched: "warning", hold: "warning", invalid: "danger" };
const KNOWN = new Set(["barcode", "auto_create", "no_barcode", "unknown_barcode", "already_linked", "zero_price", "bad_price", "over_mrp", "big_drop", "no_name"]);
const reasonText = (r: string | null) => (r === null ? "" : KNOWN.has(r) ? t(`admin.connector.reason.${r}` as MessageKey) : r);

/** Step 4: the first 20 rows as the platform would read them. Nothing is written in either database. */
export function PreviewCard({ connector }: { connector: Connector }) {
  const preview = usePreview(connector.id);
  const columns: Column<PreviewRow>[] = [
    { key: "name", header: t("admin.connector.col.name"), cell: (r) => <span><span className="font-semibold">{r.name ?? "–"}</span><span className="block text-xs text-muted">{r.externalId}</span></span> },
    { key: "price", header: t("admin.connector.col.price"), align: "right", cell: (r) => (r.priceCents === null ? "–" : formatLKR(r.priceCents)) },
    { key: "action", header: t("admin.connector.action"), cell: (r) => <Badge tone={TONE[r.action]}>{t(`admin.connector.act.${r.action}` as MessageKey)}</Badge> },
    { key: "reason", header: t("admin.connector.why"), cell: (r) => reasonText(r.reason), hideOnMobile: true },
  ];
  return (
    <Card className="flex flex-col gap-3 p-4">
      <h2 className="text-lg font-semibold">{t("admin.connector.stepPreview")}</h2>
      <Button variant="secondary" className="self-start" loading={preview.isPending} disabled={!connector.sourceTable} onClick={() => preview.mutate(undefined)}>{t("admin.connector.previewRun")}</Button>
      {preview.error && <Banner tone="danger">{errorText(preview.error)}</Banner>}
      {preview.data && <DataTable caption={t("admin.connector.stepPreview")} columns={columns} rows={preview.data.rows} rowKey={(r) => r.externalId} emptyTitle={t("admin.connector.previewEmpty")} />}
    </Card>
  );
}
