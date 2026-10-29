"use client";

import clsx from "clsx";
import Link from "next/link";
import type { ReactNode } from "react";
import type { Address } from "@/features/address/types";
import { t } from "@/lib/i18n";
import { DestinationForm } from "./DestinationForm";
import type { Destination } from "./types";

export type WhereChoice = { kind: "saved"; id: string } | { kind: "other" };

interface Props {
  addresses: readonly Address[];
  choice: WhereChoice;
  onChoice: (c: WhereChoice) => void;
  onDestination: (d: Destination | null) => void;
}

function Option({ checked, onSelect, children }: { checked: boolean; onSelect: () => void; children: ReactNode }) {
  return (
    <label className={clsx("flex cursor-pointer gap-3 rounded-card border p-3", checked ? "border-ink" : "border-line")}>
      <input type="radio" name="where" checked={checked} onChange={onSelect} className="mt-1 size-4 accent-[var(--c-text)]" />
      <div className="min-w-0 flex-1">{children}</div>
    </label>
  );
}

/** Where to deliver: one of the saved addresses, or another place for this order only. */
export function WherePicker({ addresses, choice, onChoice, onDestination }: Props) {
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="mb-2 text-lg font-semibold">{t("checkout.where")}</legend>
      {addresses.map((a) => (
        <Option key={a.id} checked={choice.kind === "saved" && choice.id === a.id} onSelect={() => onChoice({ kind: "saved", id: a.id })}>
          <div className="font-semibold">{a.label}{a.isDefault && <span className="ml-2 text-sm font-normal text-muted">{t("checkout.main")}</span>}</div>
          <div className="text-sm text-muted">{a.line1}{a.landmark ? `, ${a.landmark}` : ""}</div>
        </Option>
      ))}
      <Option checked={choice.kind === "other"} onSelect={() => onChoice({ kind: "other" })}>
        <div className="font-semibold">{t("checkout.other")}</div>
        <div className="text-sm text-muted">{t("checkout.otherHint")}</div>
        {choice.kind === "other" && <DestinationForm onChange={onDestination} />}
      </Option>
      <Link href="/account/addresses" className="text-sm font-semibold underline">{t("checkout.manageAddresses")}</Link>
    </fieldset>
  );
}
