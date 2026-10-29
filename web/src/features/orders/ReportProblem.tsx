"use client";

import { useState } from "react";
import { Banner, Button, Field, Modal, Select, Textarea } from "@/components/ui";
import { ticketReasonValues, type TicketReason } from "@/lib/enums.gen";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import { useReportProblem } from "./api";
import { ticketText } from "./statusText";

export function ReportProblem({ orderId, open, onClose }: { orderId: string; open: boolean; onClose: () => void }) {
  const report = useReportProblem(orderId);
  const [reason, setReason] = useState<TicketReason>("MISSING_ITEM");
  const [note, setNote] = useState("");

  const close = () => { report.reset(); setNote(""); onClose(); };
  return (
    <Modal open={open} onClose={close} title={t("order.reportTitle")}
      footer={report.isSuccess
        ? <Button className="flex-1" onClick={close}>{t("common.done")}</Button>
        : <Button className="flex-1" loading={report.isPending} onClick={() => report.mutate({ reason, note })}>{t("order.reportSend")}</Button>}>
      {report.isSuccess ? (
        <Banner tone="success">{t("order.reportDone")}</Banner>
      ) : (
        <div className="flex flex-col gap-3">
          <Field label={t("order.reportReason")}>
            {(p) => (
              <Select {...p} value={reason} onChange={(e) => setReason(e.target.value as TicketReason)}>
                {ticketReasonValues.map((r) => <option key={r} value={r}>{t(ticketText[r])}</option>)}
              </Select>
            )}
          </Field>
          <Field label={t("order.reportNote")} error={report.error ? errorText(report.error) : undefined}>
            {(p) => <Textarea {...p} maxLength={500} rows={3} value={note} onChange={(e) => setNote(e.target.value)} />}
          </Field>
        </div>
      )}
    </Modal>
  );
}
