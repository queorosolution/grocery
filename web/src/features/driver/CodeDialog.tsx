"use client";

import { useState } from "react";
import { Banner, Button, Field, Input, Sheet, useToast } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import { useComplete } from "./api";
import type { Delivery } from "./types";

/** Five wrong tries lock the order (docs/06 section 9). The server counts; this only counts what it saw. */
export const MAX_TRIES = 5;
const CODE_LENGTH = 4;

interface Props {
  delivery: Delivery;
  /** Wrong codes sent for this order since the screen opened. */
  wrong: number;
  onWrong: (total: number) => void;
  open: boolean;
  onClose: () => void;
}

/** The customer's 4-digit code, typed big. Wrong, locked and other errors each have their own words. */
export function CodeDialog({ delivery, wrong, onWrong, open, onClose }: Props) {
  const complete = useComplete(delivery.id);
  const toast = useToast();
  const [code, setCode] = useState("");
  const left = Math.max(0, MAX_TRIES - wrong);
  const locked = left === 0;
  const failure = complete.error;
  const otherError = failure && !(failure instanceof ApiError && failure.code === "DELIVERY_CODE_INVALID") ? errorText(failure) : null;

  const close = () => { complete.reset(); setCode(""); onClose(); };
  const send = () => {
    if (code.length !== CODE_LENGTH || locked) return;
    complete.mutate({ version: delivery.version, code }, {
      onSuccess: () => { toast.show(t("driver.doneNote")); close(); },
      onError: (err) => {
        setCode("");
        if (!(err instanceof ApiError)) return;
        if (err.code === "DELIVERY_CODE_INVALID") onWrong(wrong + 1);
        if (err.code === "DELIVERY_CODE_LOCKED") onWrong(MAX_TRIES);
      },
    });
  };

  return (
    <Sheet open={open} onClose={close} title={t("driver.codeTitle")}
      footer={<Button className="h-12 flex-1" loading={complete.isPending} disabled={locked || code.length !== CODE_LENGTH} onClick={send}>{t("driver.confirm")}</Button>}>
      <div className="flex flex-col gap-3">
        <p className="text-muted">{t("driver.codeBody", { name: delivery.customer.name })}</p>
        <Field label={t("driver.codeLabel")}>
          {(p) => (
            <Input {...p} value={code} disabled={locked} inputMode="numeric" pattern="[0-9]*" maxLength={CODE_LENGTH} autoComplete="off"
              className="h-16 text-center text-3xl font-semibold tracking-[0.5em]"
              onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
              onKeyDown={(e) => { if (e.key === "Enter") send(); }} />
          )}
        </Field>
        <div role="status" aria-live="polite">
          {locked ? <Banner tone="danger">{t("driver.locked")}</Banner>
            : otherError ? <Banner tone="danger">{otherError}</Banner>
            : wrong > 0 && <Banner tone="warning">{left === 1 ? t("driver.wrongCodeOne") : t("driver.wrongCode", { left })}</Banner>}
        </div>
      </div>
    </Sheet>
  );
}
