"use client";

import { useState } from "react";
import { Banner, Button, Card, Field, Modal, Textarea } from "@/components/ui";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import { useReview } from "./api";
import { ReviseEditor } from "./ReviseEditor";
import type { ShopOrder } from "./types";

const MIN_REASON = 3; // docs/06 section 8

/** First look at a new bill: approve it, change it, or decline it. Each button is there because the server listed it. */
export function ReviewPanel({ shopId, order }: { shopId: string; order: ShopOrder }) {
  const review = useReview(shopId, order.id);
  const [revising, setRevising] = useState(false);
  const [rejecting, setRejecting] = useState(false);
  const [reason, setReason] = useState("");
  const can = (a: string) => order.allowedActions.includes(a);
  if (!can("APPROVE") && !can("REVISE") && !can("REJECT")) return null;

  return (
    <Card className="flex flex-col gap-2 p-4">
      <h2 className="text-lg font-semibold">{t("shopConsole.reviewTitle")}</h2>
      <p className="text-sm text-muted">{t("shopConsole.reviewBody")}</p>
      {can("APPROVE") && (
        <Button size="lg" loading={review.isPending && review.variables?.action === "APPROVE"}
          onClick={() => review.mutate({ version: order.version, action: "APPROVE" })}>{t("shopConsole.approve")}</Button>
      )}
      {can("REVISE") && <Button variant="secondary" onClick={() => setRevising(true)}>{t("shopConsole.revise")}</Button>}
      {can("REJECT") && <Button variant="ghost" onClick={() => setRejecting(true)}>{t("shopConsole.reject")}</Button>}
      {review.error && !revising && !rejecting && <Banner tone="danger">{errorText(review.error)}</Banner>}
      <ReviseEditor shopId={shopId} order={order} open={revising} onClose={() => setRevising(false)} />
      <Modal open={rejecting} onClose={() => setRejecting(false)} title={t("shopConsole.rejectTitle")}
        footer={
          <Button variant="danger" className="flex-1" loading={review.isPending} disabled={reason.trim().length < MIN_REASON}
            onClick={() => review.mutate({ version: order.version, action: "REJECT", reason: reason.trim() }, { onSuccess: () => setRejecting(false) })}>
            {t("shopConsole.reject")}
          </Button>
        }>
        <Field label={t("shopConsole.reason")} hint={t("shopConsole.rejectHint")} error={review.error ? errorText(review.error) : undefined}>
          {(p) => <Textarea {...p} maxLength={300} value={reason} onChange={(e) => setReason(e.target.value)} />}
        </Field>
      </Modal>
    </Card>
  );
}
