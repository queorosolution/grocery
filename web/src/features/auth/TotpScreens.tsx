"use client";

import { useState, type FormEvent } from "react";
import { Banner, Button, Field, Input, QrCode } from "@/components/ui";
import { Page } from "@/features/shell/Page";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import { useTotpSetup, useTotpVerify } from "./api";

/** The 6-digit box shared by both screens. A wrong code shows the API's own message. */
function CodeForm({ submitLabel }: { submitLabel: string }) {
  const verify = useTotpVerify();
  const [code, setCode] = useState("");
  const submit = (e: FormEvent) => {
    e.preventDefault();
    verify.mutate(code);
  };
  return (
    <form onSubmit={submit} className="flex flex-col gap-3">
      <Field label={t("totp.codeLabel")} error={verify.error ? errorText(verify.error) : undefined}>
        {(p) => (
          <Input {...p} inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={code}
            onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 6))} />
        )}
      </Field>
      <Button type="submit" loading={verify.isPending} disabled={code.length !== 6}>{submitLabel}</Button>
    </form>
  );
}

/** First sign-in of a super admin: show the QR, then take the first code (which turns TOTP on). */
export function TotpSetupScreen() {
  const setup = useTotpSetup();
  const data = setup.data;
  // Only an authenticator link may become a QR or an href.
  const uri = data?.otpauthUri.startsWith("otpauth://totp/") ? data.otpauthUri : null;
  return (
    <Page className="max-w-md">
      <h1 className="text-xl font-semibold">{t("totp.setupTitle")}</h1>
      <p className="mt-1 text-muted">{t("totp.setupBody")}</p>
      <div className="mt-4 flex flex-col gap-4">
        {setup.error && <Banner tone="danger">{errorText(setup.error)}</Banner>}
        {!uri ? (
          <Button loading={setup.isPending} onClick={() => setup.mutate()}>{t("totp.start")}</Button>
        ) : (
          <>
            <div className="flex flex-col items-center gap-2 rounded-card border border-line p-4">
              <QrCode value={uri} label={t("totp.qrLabel")} />
              <p className="text-sm text-muted">{t("totp.secret", { secret: data?.secretMasked ?? "" })}</p>
              <a href={uri} className="text-sm font-semibold text-accent underline">{t("totp.openApp")}</a>
            </div>
            <CodeForm submitLabel={t("totp.turnOn")} />
          </>
        )}
      </div>
    </Page>
  );
}

/** Every new sign-in of a super admin needs a code before the admin screens open. */
export function TotpCodeScreen() {
  return (
    <Page className="max-w-md">
      <h1 className="text-xl font-semibold">{t("totp.codeTitle")}</h1>
      <p className="mt-1 mb-4 text-muted">{t("totp.codeBody")}</p>
      <CodeForm submitLabel={t("totp.verify")} />
    </Page>
  );
}
