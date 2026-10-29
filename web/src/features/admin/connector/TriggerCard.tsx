"use client";

import { Copy } from "lucide-react";
import { Banner, Button, Card, Modal, useToast } from "@/components/ui";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import type { Connector } from "../types";
import { useRotateTrigger } from "./connectorApi";

/**
 * Step 6: the secret that signs the shop's "price changed" calls. It is shown once, here, and dropped
 * from memory when the window closes. Making a new one stops the old one at once.
 */
export function TriggerCard({ connector }: { connector: Connector }) {
  const rotate = useRotateTrigger(connector.id);
  const toast = useToast();
  const copy = (text: string) => { void navigator.clipboard?.writeText(text).then(() => toast.show(t("admin.connector.copied")), () => undefined); };
  return (
    <Card className="flex flex-col gap-3 p-4">
      <h2 className="text-lg font-semibold">{t("admin.connector.stepTrigger")}</h2>
      <p className="text-muted">{t("admin.connector.triggerBody")}</p>
      <Button variant="secondary" className="self-start" loading={rotate.isPending} onClick={() => rotate.mutate(undefined)}>{t("admin.connector.triggerMake")}</Button>
      {rotate.error && <Banner tone="danger">{errorText(rotate.error)}</Banner>}
      <Modal open={!!rotate.data} onClose={() => rotate.reset()} title={t("admin.connector.triggerTitle")}
        footer={<Button className="flex-1" onClick={() => rotate.reset()}>{t("admin.connector.triggerDone")}</Button>}>
        {rotate.data && (
          <div className="flex flex-col gap-3">
            <Banner tone="warning">{t("admin.connector.triggerOnce")}</Banner>
            <div className="flex items-center gap-2">
              <code className="min-w-0 flex-1 break-all rounded-input bg-surface p-2 text-sm">{rotate.data.secret}</code>
              <Button variant="secondary" aria-label={t("admin.connector.copy")} onClick={() => copy(rotate.data.secret)}><Copy size={16} aria-hidden="true" /></Button>
            </div>
            <pre className="overflow-x-auto rounded-input bg-surface p-2 text-xs">{rotate.data.example}</pre>
          </div>
        )}
      </Modal>
    </Card>
  );
}
