"use client";

import { useState } from "react";
import { Banner, Button, Modal } from "@/components/ui";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import { submitPayHere } from "@/lib/payhere";
import { useStartPayment } from "./api";
import type { OrderDetail, PaymentStart } from "./types";

type Bank = Extract<PaymentStart, { provider: "MANUAL" }>;

function BankDetails({ bank, onClose }: { bank: Bank; onClose: () => void }) {
  const rows: [string, string][] = [
    [t("order.bankName"), bank.bank.bankName], [t("order.accountName"), bank.bank.accountName],
    [t("order.accountNumber"), bank.bank.accountNumber], [t("order.branch"), bank.bank.branch], [t("order.bankRef"), bank.reference],
  ];
  return (
    <Modal open onClose={onClose} title={t("order.bankTitle")} footer={<Button className="flex-1" onClick={onClose}>{t("common.done")}</Button>}>
      <p className="mb-3">{t("order.bankBody", { amount: formatLKR(bank.amountCents) })}</p>
      <dl className="flex flex-col gap-2">
        {rows.map(([label, value]) => (
          <div key={label}><dt className="text-sm text-muted">{label}</dt><dd className="font-semibold">{value}</dd></div>
        ))}
      </dl>
    </Modal>
  );
}

/**
 * Starts a payment. It renders only when the server lists PAY for this order (D29); what the server
 * answers is used as it is: PayHere's form is built and checked in lib/payhere.ts, and a bank
 * transfer shows the details. Nothing here decides from `status`.
 */
export function PayButton({ order }: { order: OrderDetail }) {
  const start = useStartPayment(order.id);
  const [bank, setBank] = useState<Bank | null>(null);
  const [sending, setSending] = useState(false);
  const [failed, setFailed] = useState<string | null>(null);

  if (!order.allowedActions.includes("PAY")) return null;

  const pay = () => {
    setFailed(null);
    start.mutate(order.version, {
      onSuccess: (p) => {
        if (p.provider === "MANUAL") return setBank(p);
        try {
          setSending(true);
          submitPayHere(p.action, p.fields);
        } catch {
          setSending(false);
          setFailed(t("order.payFailed"));
        }
      },
    });
  };

  return (
    <>
      <Button size="lg" block loading={start.isPending || sending} onClick={pay}>{sending ? t("order.paying") : t("order.pay")}</Button>
      {(start.error || failed) && <Banner tone="danger">{failed ?? errorText(start.error)}</Banner>}
      {bank && <BankDetails bank={bank} onClose={() => setBank(null)} />}
    </>
  );
}
