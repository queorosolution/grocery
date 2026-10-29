"use client";

import { Button, EmptyState, Skeleton } from "@/components/ui";
import { t } from "@/lib/i18n";
import type { ShopDetail } from "../types";
import { ConnectCard } from "./ConnectCard";
import { useConnector } from "./connectorApi";
import { MappingCard } from "./MappingCard";
import { PreviewCard } from "./PreviewCard";
import { RunsCard } from "./RunsCard";
import { StatusCard } from "./StatusCard";
import { TriggerCard } from "./TriggerCard";

/** The connector wizard: connect → test → table → map → preview → activate → trigger secret. */
export function ConnectorTab({ detail }: { detail: ShopDetail }) {
  const id = detail.shop.id;
  const connector = useConnector(id);
  if (connector.isPending) return <Skeleton className="h-40" />;
  if (connector.isError) return <EmptyState title={t("common.error")} action={<Button onClick={() => void connector.refetch()}>{t("common.retry")}</Button>} />;
  const c = connector.data ?? null;
  return (
    <div className="flex flex-col gap-4">
      <ConnectCard shopId={id} connector={c} />
      {c && (
        <>
          <MappingCard key={`${c.id}-${c.sourceTable ?? ""}`} shopId={id} connector={c} />
          <PreviewCard connector={c} />
          <StatusCard shopId={id} connector={c} />
          <TriggerCard connector={c} />
          <RunsCard connector={c} />
        </>
      )}
    </div>
  );
}
