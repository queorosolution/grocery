"use client";

import { useState } from "react";
import { Button, Field, Input, Modal, Select } from "@/components/ui";
import { paymentAccountStatusValues, paymentModeValues, paymentProviderValues, type PaymentProvider } from "@/lib/enums.gen";
import { t } from "@/lib/i18n";
import { fieldText, SaveNotice } from "../form";
import type { Bank, PaymentAccount } from "../types";
import { useCreateAccount, useUpdateAccount } from "./paymentApi";

const NO_BANK: Bank = { bankName: "", accountName: "", accountNumber: "", branch: "" };
const BANK_FIELDS = ["bankName", "accountName", "accountNumber", "branch"] as const;

interface Props {
  shopId: string;
  /** The account being changed, or null to add one. */
  account: PaymentAccount | null;
  open: boolean;
  onClose: () => void;
}

/**
 * Add or change a payment account. The PayHere merchant secret is write-only: it is sent once, the box
 * empties as soon as the save works, and the server never sends it back (only `hasSecret`).
 */
export function AccountDialog({ shopId, account, open, onClose }: Props) {
  const create = useCreateAccount();
  const update = useUpdateAccount(account?.id ?? "");
  const mutation = account ? update : create;
  const [provider, setProvider] = useState<PaymentProvider>(account?.provider ?? "PAYHERE");
  const [mode, setMode] = useState(account?.mode ?? "SANDBOX");
  const [status, setStatus] = useState(account?.status ?? "ACTIVE");
  const [merchantId, setMerchantId] = useState(account?.merchantId ?? "");
  const [secret, setSecret] = useState("");
  const [bank, setBank] = useState<Bank>(account?.bank ?? NO_BANK);
  const payhere = provider === "PAYHERE";
  const bankOk = BANK_FIELDS.slice(0, 3).every((k) => bank[k].trim().length > 0);
  const valid = payhere ? merchantId.trim().length > 0 && (account !== null || secret.length > 0) : bankOk;

  const close = () => { mutation.reset(); setSecret(""); onClose(); };
  const submit = () => {
    const trimmed = Object.fromEntries(BANK_FIELDS.map((k) => [k, bank[k].trim()])) as unknown as Bank;
    const done = { onSuccess: close };
    if (account) {
      update.mutate({ mode, status, ...(payhere ? { merchantId: merchantId.trim(), ...(secret ? { merchantSecret: secret } : {}) } : { bank: trimmed }) }, done);
    } else {
      create.mutate({ ownerType: "SHOP", shopId, provider, mode, merchantId: payhere ? merchantId.trim() : "", merchantSecret: payhere ? secret : "", bank: payhere ? null : trimmed }, done);
    }
  };

  return (
    <Modal open={open} onClose={close} title={account ? t("admin.account.edit") : t("admin.account.add")}
      footer={<Button className="flex-1" loading={mutation.isPending} disabled={!valid} onClick={submit}>{t("common.save")}</Button>}>
      <div className="flex flex-col gap-3">
        {!account && (
          <Field label={t("admin.settings.paymentProvider")}>
            {(p) => <Select {...p} value={provider} onChange={(e) => setProvider(e.target.value as PaymentProvider)}>{paymentProviderValues.map((v) => <option key={v} value={v}>{t(`admin.payment.provider_${v}`)}</option>)}</Select>}
          </Field>
        )}
        <Field label={t("admin.account.mode")}>{(p) => <Select {...p} value={mode} onChange={(e) => setMode(e.target.value as typeof mode)}>{paymentModeValues.map((v) => <option key={v} value={v}>{t(`admin.account.mode_${v}`)}</option>)}</Select>}</Field>
        {account && <Field label={t("admin.shops.status")}>{(p) => <Select {...p} value={status} onChange={(e) => setStatus(e.target.value as typeof status)}>{paymentAccountStatusValues.map((v) => <option key={v} value={v}>{t(`admin.account.status_${v}`)}</option>)}</Select>}</Field>}
        {payhere ? (
          <>
            <Field label={t("admin.account.merchantId")} error={fieldText(mutation.error, "merchantId")}>{(p) => <Input {...p} value={merchantId} autoComplete="off" onChange={(e) => setMerchantId(e.target.value)} />}</Field>
            <Field label={t("admin.account.secret")} hint={account?.hasSecret ? t("admin.account.secretKeep") : t("admin.account.secretHint")} error={fieldText(mutation.error, "merchantSecret")}>
              {(p) => <Input {...p} type="password" value={secret} autoComplete="new-password" onChange={(e) => setSecret(e.target.value)} />}
            </Field>
          </>
        ) : BANK_FIELDS.map((k) => (
          <Field key={k} label={t(`admin.account.${k}`)} error={fieldText(mutation.error, k) ?? fieldText(mutation.error, "bank")}>
            {(p) => <Input {...p} maxLength={80} value={bank[k]} onChange={(e) => setBank((b) => ({ ...b, [k]: e.target.value }))} />}
          </Field>
        ))}
        <SaveNotice error={mutation.error} saved={false} />
      </div>
    </Modal>
  );
}
