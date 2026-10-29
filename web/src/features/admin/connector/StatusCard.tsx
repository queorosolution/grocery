"use client";

import { Banner, Badge, Button, Card, type Tone } from "@/components/ui";
import { errorText } from "@/lib/errorText";
import { t, type MessageKey } from "@/lib/i18n";
import { formatDateTime } from "@/lib/time";
import type { Connector } from "../types";
import { useSetLive, useSyncNow } from "./connectorApi";

const TONE: Record<Connector["status"], Tone> = { DRAFT: "neutral", ACTIVE: "success", PAUSED: "warning", FAILING: "danger" };

/** Step 5: switch the sync on or off, or run it once now. A live connector (ACTIVE or FAILING) has a schedule. */
export function StatusCard({ shopId, connector }: { shopId: string; connector: Connector }) {
  const live = connector.status === "ACTIVE" || connector.status === "FAILING";
  const toggle = useSetLive(shopId, connector.id);
  const sync = useSyncNow(connector.id);
  const error = toggle.error ?? sync.error;
  return (
    <Card className="flex flex-col gap-3 p-4">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-lg font-semibold">{t("admin.connector.stepActivate")}</h2>
        <Badge tone={TONE[connector.status]}>{t(`admin.connector.status.${connector.status}` as MessageKey)}</Badge>
      </div>
      <p className="text-sm text-muted">{connector.lastRunAt ? t("admin.connector.lastRun", { at: formatDateTime(connector.lastRunAt) }) : t("admin.connector.neverRun")}</p>
      <div className="flex flex-wrap gap-2">
        <Button variant={live ? "secondary" : "primary"} loading={toggle.isPending} disabled={!connector.sourceTable} onClick={() => toggle.mutate(!live)}>
          {live ? t("admin.connector.pause") : t("admin.connector.activate")}
        </Button>
        <Button variant="secondary" loading={sync.isPending} disabled={!connector.sourceTable} onClick={() => sync.mutate(undefined)}>{t("admin.connector.syncNow")}</Button>
      </div>
      {sync.isSuccess && <p role="status" className="text-sm text-success">{t("admin.connector.syncQueued")}</p>}
      {error && <Banner tone="danger">{errorText(error)}</Banner>}
    </Card>
  );
}
