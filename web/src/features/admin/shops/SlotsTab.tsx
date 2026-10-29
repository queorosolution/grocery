"use client";

import { useState } from "react";
import { Badge, Banner, Button, ConfirmDialog, EmptyState, Input, Skeleton } from "@/components/ui";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import { addDays, colomboDay, formatDay, formatTime } from "@/lib/time";
import type { ShopDetail, Slot } from "../types";
import { SlotDialog } from "./SlotDialog";
import { useCopyWeek, useSlots, useUpdateSlot } from "./slotsApi";

const DAYS = 7;

function SlotRow({ slot, onChange, busy }: { slot: Slot; onChange: (p: { capacity?: number; status?: Slot["status"] }) => void; busy: boolean }) {
  const [cap, setCap] = useState(String(slot.capacity));
  const open = slot.status === "OPEN";
  return (
    <li className="flex flex-wrap items-center gap-2 px-3 py-2">
      <span className="min-w-36 flex-1 font-semibold">{formatTime(slot.startsAt)} – {formatTime(slot.endsAt)}</span>
      <span className="text-sm text-muted">{t("admin.slots.booked", { booked: slot.booked, capacity: slot.capacity })}</span>
      <Input aria-label={t("admin.slots.capacity")} className="w-20" inputMode="numeric" value={cap} onChange={(e) => setCap(e.target.value)}
        onBlur={() => { const n = /^\d{1,3}$/.test(cap) ? Number(cap) : null; if (n === null) setCap(String(slot.capacity)); else if (n !== slot.capacity) onChange({ capacity: n }); }} />
      <Button variant="secondary" disabled={busy} onClick={() => onChange({ status: open ? "CLOSED" : "OPEN" })}>{open ? t("admin.slots.close") : t("admin.slots.reopen")}</Button>
      {!open && <Badge>{t("admin.slots.closed")}</Badge>}
    </li>
  );
}

/** The next 7 days of slots for the week shown, grouped by day. Only for shops that deliver in slots. */
export function SlotsTab({ detail }: { detail: ShopDetail }) {
  const id = detail.shop.id;
  const [weekStart, setWeekStart] = useState(() => colomboDay(0));
  const [adding, setAdding] = useState(false);
  const [copying, setCopying] = useState(false);
  const slots = useSlots(id, weekStart, addDays(weekStart, DAYS - 1));
  const update = useUpdateSlot(id);
  const copy = useCopyWeek(id);
  const days = Array.from({ length: DAYS }, (_, i) => addDays(weekStart, i));
  const byDay = (d: string) => (slots.data?.data ?? []).filter((s) => new Date(s.startsAt).toLocaleDateString("en-CA", { timeZone: "Asia/Colombo" }) === d);

  if (detail.settings.deliveryType !== "SLOT") return <EmptyState title={t("admin.slots.courier")} />;
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="secondary" onClick={() => setWeekStart(addDays(weekStart, -DAYS))} disabled={weekStart <= colomboDay(0)}>{t("admin.slots.prev")}</Button>
        <Button variant="secondary" onClick={() => setWeekStart(addDays(weekStart, DAYS))}>{t("admin.slots.next")}</Button>
        <span className="flex-1" />
        <Button variant="secondary" onClick={() => setCopying(true)}>{t("admin.slots.copy")}</Button>
        <Button onClick={() => setAdding(true)}>{t("admin.slots.new")}</Button>
      </div>
      {(update.error || copy.error) && <Banner tone="danger">{errorText(update.error ?? copy.error)}</Banner>}
      {copy.data && <p role="status" className="text-sm text-success">{t("admin.slots.copied", { count: copy.data.created })}</p>}
      {slots.isPending ? <Skeleton className="h-40" /> : slots.isError ? <EmptyState title={t("common.error")} /> : days.map((d) => (
        <section key={d}>
          <h2 className="mb-1 text-base font-semibold">{formatDay(`${d}T12:00:00+05:30`)}</h2>
          {byDay(d).length === 0 ? <p className="text-sm text-muted">{t("admin.slots.none")}</p> : (
            <ul className="divide-y divide-line rounded-card border border-line">
              {byDay(d).map((s) => <SlotRow key={`${s.id}-${s.capacity}-${s.status}`} slot={s} busy={update.isPending} onChange={(p) => update.mutate({ id: s.id, ...p })} />)}
            </ul>
          )}
        </section>
      ))}
      <SlotDialog shopId={id} open={adding} onClose={() => setAdding(false)} />
      <ConfirmDialog open={copying} title={t("admin.slots.copy")} message={t("admin.slots.copyBody", { from: weekStart })} loading={copy.isPending}
        onClose={() => setCopying(false)} onConfirm={() => copy.mutate(weekStart, { onSuccess: () => setCopying(false) })} />
    </div>
  );
}
