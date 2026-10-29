"use client";

import { useState } from "react";
import { Button } from "@/components/ui";
import { t, type MessageKey } from "@/lib/i18n";
import { centsToRupeesText, parseRupees } from "@/lib/money";
import { DecimalField, MoneyField } from "../fields";
import { fieldText, parseDec2, SaveNotice } from "../form";
import type { FeeRule } from "../types";

const MONEY = ["baseCents", "perKmCents", "perKgCents", "minCents"] as const;
const OPTIONAL_MONEY = ["freeAboveCents", "maxCents"] as const;
const DECIMAL = ["freeKm", "freeKg", "maxKm", "roadFactor"] as const;

type Draft = Record<(typeof MONEY)[number] | (typeof OPTIONAL_MONEY)[number] | (typeof DECIMAL)[number], string>;

const label = (k: string) => t(`admin.fees.${k}` as MessageKey);

function toDraft(r: FeeRule): Draft {
  return {
    ...Object.fromEntries(MONEY.map((k) => [k, centsToRupeesText(r[k])])),
    ...Object.fromEntries(OPTIONAL_MONEY.map((k) => [k, r[k] === null ? "" : centsToRupeesText(r[k])])),
    ...Object.fromEntries(DECIMAL.map((k) => [k, String(r[k])])),
  } as Draft;
}

/** Turns the typed text into a rule, or null when a box is not a number. Money goes through parseRupees: exact cents. */
export function toRule(d: Draft): FeeRule | null {
  const money = MONEY.map((k) => parseRupees(d[k]));
  const optional = OPTIONAL_MONEY.map((k) => (d[k].trim() === "" ? undefined : parseRupees(d[k])));
  const dec = DECIMAL.map((k) => parseDec2(d[k]));
  if (money.includes(null) || optional.includes(null) || dec.includes(null)) return null;
  const [baseCents, perKmCents, perKgCents, minCents] = money as number[];
  const [freeKm, freeKg, maxKm, roadFactor] = dec as number[];
  return {
    baseCents: baseCents!, perKmCents: perKmCents!, perKgCents: perKgCents!, minCents: minCents!,
    freeAboveCents: optional[0] ?? null, maxCents: optional[1] ?? null,
    freeKm: freeKm!, freeKg: freeKg!, maxKm: maxKm!, roadFactor: roadFactor!,
  };
}

interface Props {
  initial: FeeRule;
  pending: boolean;
  error: unknown;
  saved: boolean;
  submitLabel: string;
  onSave: (rule: FeeRule) => void;
}

/** The delivery fee rule: rupees for money, numbers with 2 decimals for km and kg. A blank limit means no limit. */
export function RuleForm({ initial, pending, error, saved, submitLabel, onSave }: Props) {
  const [d, setD] = useState(() => toDraft(initial));
  const rule = toRule(d);
  const set = (k: keyof Draft) => (v: string) => setD((o) => ({ ...o, [k]: v }));
  return (
    <form className="grid gap-3 md:grid-cols-2" onSubmit={(e) => { e.preventDefault(); if (rule) onSave(rule); }}>
      {MONEY.map((k) => <MoneyField key={k} label={label(k)} value={d[k]} onChange={set(k)} error={fieldText(error, k)} />)}
      {OPTIONAL_MONEY.map((k) => <MoneyField key={k} label={label(k)} hint={t("admin.fees.blank")} value={d[k]} onChange={set(k)} error={fieldText(error, k)} />)}
      {DECIMAL.map((k) => <DecimalField key={k} label={label(k)} value={d[k]} onChange={set(k)} error={fieldText(error, k)} />)}
      <div className="flex flex-col gap-2 md:col-span-2">
        <SaveNotice error={error} saved={saved} />
        <Button type="submit" loading={pending} disabled={!rule} className="self-start">{submitLabel}</Button>
      </div>
    </form>
  );
}
