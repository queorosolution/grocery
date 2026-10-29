"use client";

import { useState, type FormEvent } from "react";
import { Banner, Button, Field, Input } from "@/components/ui";
import { errorText, fieldErrors } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import { usePhoneSend, usePhoneVerify } from "./api";
import { Turnstile } from "./Turnstile";
import type { Me } from "./types";

/** Phone number, bot check, then the 6-digit SMS code. Used in onboarding, account and checkout. */
export function PhoneVerify({ onVerified }: { onVerified: (me: Me) => void }) {
  const send = usePhoneSend();
  const verify = usePhoneVerify();
  const [phone, setPhone] = useState("");
  const [code, setCode] = useState("");
  const [token, setToken] = useState<string | null>(null);
  const [sentTo, setSentTo] = useState<string | null>(null);
  const [fresh, setFresh] = useState(0);

  const sendCode = (e: FormEvent) => {
    e.preventDefault();
    if (!token) return;
    send.mutate({ phone: phone.trim(), turnstileToken: token }, {
      onSuccess: () => setSentTo(phone.trim()),
      // A token works once, so a refused send needs a new check.
      onError: () => setFresh((n) => n + 1),
    });
  };
  const checkCode = (e: FormEvent) => {
    e.preventDefault();
    verify.mutate(code, { onSuccess: onVerified });
  };

  if (sentTo) {
    return (
      <form onSubmit={checkCode} className="flex flex-col gap-3">
        <p className="text-muted">{t("phone.sent", { phone: sentTo })}</p>
        <Field label={t("phone.codeLabel")} error={verify.error ? errorText(verify.error) : undefined}>
          {(p) => (
            <Input {...p} inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 6))} />
          )}
        </Field>
        <Button type="submit" loading={verify.isPending} disabled={code.length < 4}>{t("phone.verify")}</Button>
        <Button variant="ghost" onClick={() => { setSentTo(null); setCode(""); setToken(null); verify.reset(); }}>{t("phone.change")}</Button>
      </form>
    );
  }

  const phoneError = fieldErrors(send.error, { phone: t("phone.invalid") }).phone;
  return (
    <form onSubmit={sendCode} className="flex flex-col gap-3">
      <Field label={t("phone.label")} hint={t("phone.hint")} error={phoneError}>
        {(p) => <Input {...p} type="tel" inputMode="tel" autoComplete="tel" maxLength={15} value={phone} onChange={(e) => setPhone(e.target.value)} />}
      </Field>
      <Turnstile onToken={setToken} resetKey={fresh} />
      {send.error && !phoneError && <Banner tone="danger">{errorText(send.error)}</Banner>}
      <Button type="submit" loading={send.isPending} disabled={!token || phone.trim().length < 9}>{t("phone.send")}</Button>
    </form>
  );
}
