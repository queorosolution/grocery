"use client";

import { useState } from "react";
import { Banner, Button, Card, Field, Input } from "@/components/ui";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import { fieldText } from "../form";
import type { Connector } from "../types";
import { useCreateConnector, useReplaceSecret, useTestConnector } from "./connectorApi";

interface BoxProps {
  label: string;
  pending: boolean;
  error: unknown;
  onSend: (connectionString: string) => void;
  /** Called when the string was accepted; the box is emptied by then. */
  done?: boolean;
}

/**
 * A write-only box for the connection string. It is sent once and the box empties at once; the string
 * is not shown, kept or logged anywhere after that (docs/09 section 5.5). Only the masked host comes back.
 */
function StringBox({ label, pending, error, onSend, done }: BoxProps) {
  const [value, setValue] = useState("");
  const send = () => { onSend(value.trim()); setValue(""); };
  return (
    <form className="flex flex-col gap-2" onSubmit={(e) => { e.preventDefault(); if (value.trim()) send(); }}>
      <Field label={label} hint={t("admin.connector.stringHint")} error={fieldText(error, "connectionString")}>
        {(p) => <Input {...p} type="password" autoComplete="off" spellCheck={false} placeholder="postgresql://user:password@host/db?sslmode=require" value={value} onChange={(e) => setValue(e.target.value)} />}
      </Field>
      {!!error && !fieldText(error, "connectionString") && <Banner tone="danger">{errorText(error)}</Banner>}
      {done && <p role="status" className="text-sm text-success">{t("admin.connector.stringSaved")}</p>}
      <Button type="submit" loading={pending} disabled={!value.trim()} className="self-start">{t("admin.connector.stringSend")}</Button>
    </form>
  );
}

/** Step 1: connect. With no connector it creates one; with one it shows the masked host and can replace the string and test. */
export function ConnectCard({ shopId, connector }: { shopId: string; connector: Connector | null }) {
  const create = useCreateConnector(shopId);
  const replace = useReplaceSecret(connector?.id ?? "");
  const test = useTestConnector(connector?.id ?? "");
  return (
    <Card className="flex flex-col gap-3 p-4">
      <h2 className="text-lg font-semibold">{t("admin.connector.stepConnect")}</h2>
      {connector ? (
        <>
          <p className="break-all text-sm"><span className="text-muted">{t("admin.connector.host")}: </span>{connector.host}</p>
          <StringBox label={t("admin.connector.replace")} pending={replace.isPending} error={replace.error} done={replace.isSuccess} onSend={(s) => replace.mutate(s)} />
          <div className="flex flex-col gap-2">
            <Button variant="secondary" className="self-start" loading={test.isPending} onClick={() => test.mutate(undefined)}>{t("admin.connector.test")}</Button>
            {test.data && (
              <div role="status" className="flex flex-col gap-1 text-sm">
                <span className="font-semibold text-success">{t("admin.connector.testOk", { version: test.data.serverVersion })}</span>
                {!test.data.readOnly && <Banner tone="warning">{t("admin.connector.canWrite")}</Banner>}
                {test.data.warnings.map((w) => <span key={w} className="text-muted">{w}</span>)}
              </div>
            )}
            {test.error && <Banner tone="danger">{errorText(test.error)}</Banner>}
          </div>
        </>
      ) : (
        <>
          <p className="text-muted">{t("admin.connector.none")}</p>
          <StringBox label={t("admin.connector.string")} pending={create.isPending} error={create.error} onSend={(s) => create.mutate(s)} />
        </>
      )}
    </Card>
  );
}
