"use client";

import { useState } from "react";
import { Banner, Button, Field, Modal, Select, Textarea } from "@/components/ui";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import { useRefundRequest } from "./api";
import type { ShopOrder } from "./types";

interface Props {
  shopId: string;
  order: ShopOrder;
  type: "FULL" | "PARTIAL";
  open: boolean;
  onClose: () => void;
}

const MIN_REASON = 3;

/** Ask the platform to pay money back: the whole order, or one line that could not be picked. */
export function RefundDialog({ shopId, order, type, open, onClose }: Props) {
  const request = useRefundRequest(shopId, order.id);
  const lines = order.lines.filter((l) => l.state === "ACTIVE");
  const [line, setLine] = useState("");
  const [reason, setReason] = useState("");
  const item = line || lines[0]?.id;
  const ready = reason.trim().length >= MIN_REASON && (type === "FULL" || !!item);
  const close = () => { request.reset(); setReason(""); onClose(); };

  return (
    <Modal open={open} onClose={close} title={t(type === "FULL" ? "shopConsole.refundFull" : "shopConsole.refundPartial")}
      footer={request.isSuccess
        ? <Button className="flex-1" onClick={close}>{t("common.done")}</Button>
        : <Button variant="danger" className="flex-1" loading={request.isPending} disabled={!ready}
            onClick={() => request.mutate({ type, reason: reason.trim(), ...(type === "PARTIAL" ? { orderItemId: item } : {}) })}>
            {t("shopConsole.refundSend")}
          </Button>}>
      {request.isSuccess ? (
        <Banner tone="success">{t("shopConsole.refundDone")}</Banner>
      ) : (
        <div className="flex flex-col gap-3">
          <p className="text-muted">{t(type === "FULL" ? "shopConsole.refundFullBody" : "shopConsole.refundPartialBody")}</p>
          {type === "PARTIAL" && (
            <Field label={t("shopConsole.refundLine")}>
              {(p) => <Select {...p} value={item} onChange={(e) => setLine(e.target.value)}>{lines.map((l) => <option key={l.id} value={l.id}>{l.name}</option>)}</Select>}
            </Field>
          )}
          <Field label={t("shopConsole.reason")} error={request.error ? errorText(request.error) : undefined}>
            {(p) => <Textarea {...p} maxLength={300} value={reason} onChange={(e) => setReason(e.target.value)} />}
          </Field>
        </div>
      )}
    </Modal>
  );
}
