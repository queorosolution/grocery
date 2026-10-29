"use client";

import { useState } from "react";
import { Button, Input, Switch } from "@/components/ui";
import { t, type MessageKey } from "@/lib/i18n";
import { SaveNotice } from "../form";
import type { HoursDay, ShopDetail } from "../types";
import { useSaveHours } from "./shopApi";

const DAYS: readonly MessageKey[] = ["admin.hours.d1", "admin.hours.d2", "admin.hours.d3", "admin.hours.d4", "admin.hours.d5", "admin.hours.d6", "admin.hours.d7"];
const OPENS = /^([01]\d|2[0-3]):[0-5]\d$/;
// "24:00" means midnight, and only a closing time may say it (docs/06 section 10).
const CLOSES = /^(([01]\d|2[0-3]):[0-5]\d|24:00)$/;

interface Row { open: boolean; opens: string; closes: string }

const toRows = (hours: HoursDay[]): Row[] =>
  DAYS.map((_, i) => {
    const d = hours.find((h) => h.weekday === i + 1);
    return d ? { open: true, opens: d.opens, closes: d.closes } : { open: false, opens: "08:00", closes: "21:00" };
  });

const rowOk = (r: Row) => !r.open || (OPENS.test(r.opens) && CLOSES.test(r.closes) && r.opens < r.closes);

/** The week as seven rows. A closed day has no row at all. Times are Colombo time. */
export function HoursTab({ detail }: { detail: ShopDetail }) {
  const save = useSaveHours(detail.shop.id);
  const [rows, setRows] = useState(() => toRows(detail.hours));
  const patch = (i: number, p: Partial<Row>) => setRows((all) => all.map((r, j) => (j === i ? { ...r, ...p } : r)));
  const valid = rows.every(rowOk);
  const submit = () => save.mutate(rows.flatMap((r, i) => (r.open ? [{ weekday: i + 1, opens: r.opens, closes: r.closes }] : [])));

  return (
    <form className="flex flex-col gap-2" onSubmit={(e) => { e.preventDefault(); if (valid) submit(); }}>
      <p className="text-sm text-muted">{t("admin.hours.hint")}</p>
      {rows.map((r, i) => (
        <div key={i} className="flex items-center gap-2">
          <Switch label={t(DAYS[i]!)} checked={r.open} onChange={(open) => patch(i, { open })} />
          <span className="w-24 shrink-0 font-semibold">{t(DAYS[i]!)}</span>
          {r.open ? (
            <>
              <Input aria-label={`${t(DAYS[i]!)} ${t("admin.hours.opens")}`} className="w-24" inputMode="numeric" placeholder="08:00" maxLength={5} value={r.opens} aria-invalid={!OPENS.test(r.opens) || undefined} onChange={(e) => patch(i, { opens: e.target.value })} />
              <span aria-hidden="true">–</span>
              <Input aria-label={`${t(DAYS[i]!)} ${t("admin.hours.closes")}`} className="w-24" inputMode="numeric" placeholder="21:00" maxLength={5} value={r.closes} aria-invalid={!CLOSES.test(r.closes) || r.opens >= r.closes || undefined} onChange={(e) => patch(i, { closes: e.target.value })} />
            </>
          ) : <span className="text-muted">{t("admin.hours.closed")}</span>}
        </div>
      ))}
      <SaveNotice error={save.error} saved={save.isSuccess} />
      <Button type="submit" loading={save.isPending} disabled={!valid} className="self-start">{t("admin.hours.save")}</Button>
    </form>
  );
}
