"use client";

import { useState } from "react";
import { Banner, Button, Field, Sheet, Textarea, useToast } from "@/components/ui";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import { useFail } from "./api";
import type { Delivery } from "./types";

const MIN_REASON = 3;
const MAX_REASON = 300;

/** "Couldn't deliver": the driver says why, and the shop sees the note. */
export function FailDialog({ delivery, open, onClose }: { delivery: Delivery; open: boolean; onClose: () => void }) {
  const fail = useFail(delivery.id);
  const toast = useToast();
  const [reason, setReason] = useState("");
  const close = () => { fail.reset(); setReason(""); onClose(); };
  const send = () => fail.mutate({ version: delivery.version, reason: reason.trim() }, {
    onSuccess: () => { toast.show(t("driver.failedNote")); close(); },
  });

  return (
    <Sheet open={open} onClose={close} title={t("driver.failTitle")}
      footer={<Button variant="danger" className="h-12 flex-1" loading={fail.isPending} disabled={reason.trim().length < MIN_REASON} onClick={send}>{t("driver.failSend")}</Button>}>
      <div className="flex flex-col gap-3">
        <p className="text-muted">{t("driver.failBody")}</p>
        <Field label={t("driver.reason")} hint={t("driver.reasonHint")}>
          {(p) => <Textarea {...p} maxLength={MAX_REASON} value={reason} onChange={(e) => setReason(e.target.value)} />}
        </Field>
        {fail.error && <Banner tone="danger">{errorText(fail.error)}</Banner>}
      </div>
    </Sheet>
  );
}
