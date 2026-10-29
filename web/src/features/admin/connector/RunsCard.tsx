"use client";

import { Badge, Card, EmptyState, Skeleton, type Tone } from "@/components/ui";
import { t, type MessageKey } from "@/lib/i18n";
import { formatDateTime } from "@/lib/time";
import type { Connector, SyncRun } from "../types";
import { useRuns } from "./connectorApi";

const TONE: Record<SyncRun["status"], Tone> = { RUNNING: "info", SUCCESS: "success", FAILED: "danger", SKIPPED: "neutral" };

/** The last 50 runs. The error is one of three fixed words; text from the shop's database is never shown. */
export function RunsCard({ connector }: { connector: Connector }) {
  const runs = useRuns(connector.id);
  return (
    <Card className="flex flex-col gap-2 p-4">
      <h2 className="text-lg font-semibold">{t("admin.connector.runs")}</h2>
      {runs.isPending ? <Skeleton className="h-16" /> : runs.isError ? <EmptyState title={t("common.error")} /> : runs.data.data.length === 0 ? <p className="text-muted">{t("admin.connector.noRuns")}</p> : (
        <ul className="divide-y divide-line">
          {runs.data.data.map((r) => (
            <li key={r.id} className="flex flex-col gap-1 py-2 text-sm">
              <div className="flex items-center gap-2">
                <Badge tone={TONE[r.status]}>{t(`admin.connector.run.${r.status}` as MessageKey)}</Badge>
                <span className="text-muted">{t(`admin.connector.trigger.${r.trigger}` as MessageKey)} · {formatDateTime(r.startedAt)}</span>
              </div>
              <span>{t("admin.connector.runCounts", { read: r.rowsRead, changed: r.rowsChanged, unmatched: r.rowsUnmatched, held: r.rowsHeld })}</span>
              {r.error && <span className="text-danger">{t(`admin.connector.error.${r.error}` as MessageKey)}</span>}
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}
