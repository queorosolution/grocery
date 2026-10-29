"use client";

import { useState } from "react";
import { Banner, Button, ConfirmDialog } from "@/components/ui";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import { useAcceptRevision, useCancelOrder } from "./api";
import { ReportProblem } from "./ReportProblem";
import type { OrderDetail } from "./types";

/**
 * The customer's buttons. Each one exists only because the server named it in `allowedActions`;
 * the page never works out from the status what is allowed (docs/08 section 5 rule 9).
 */
export function ActionBar({ order }: { order: OrderDetail }) {
  const cancel = useCancelOrder(order.id);
  const accept = useAcceptRevision(order.id);
  const [asking, setAsking] = useState(false);
  const [reporting, setReporting] = useState(false);
  const can = (action: string) => order.allowedActions.includes(action);
  const error = cancel.error ?? accept.error;

  return (
    <div className="flex flex-col gap-2">
      {can("ACCEPT_REVISION") && (
        <Button size="lg" block loading={accept.isPending}
          onClick={() => accept.mutate({ version: order.version, revisionNo: order.revisionNo })}>{t("order.accept")}</Button>
      )}
      {can("REPORT_PROBLEM") && <Button variant="secondary" block onClick={() => setReporting(true)}>{t("order.report")}</Button>}
      {can("CANCEL") && <Button variant="ghost" block onClick={() => setAsking(true)}>{t("order.cancel")}</Button>}
      {error && <Banner tone="danger">{errorText(error)}</Banner>}
      <ConfirmDialog open={asking} title={t("order.cancelTitle")} message={t("order.cancelBody")} confirmLabel={t("order.cancel")} danger
        loading={cancel.isPending} onClose={() => setAsking(false)}
        onConfirm={() => cancel.mutate(order.version, { onSettled: () => setAsking(false) })} />
      <ReportProblem orderId={order.id} open={reporting} onClose={() => setReporting(false)} />
    </div>
  );
}
