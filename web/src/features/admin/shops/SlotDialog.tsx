"use client";

import { useState } from "react";
import { Button, Field, Input, Modal } from "@/components/ui";
import { t } from "@/lib/i18n";
import { addDays, colomboDay, colomboIso } from "@/lib/time";
import { NumberField } from "../fields";
import { fieldText, parseInt10, SaveNotice } from "../form";
import { useCreateSlots } from "./slotsApi";

const TIME = /^([01]\d|2[0-3]):[0-5]\d$/;

/** One new slot: a day, a start and end, how long before the start orders stop, and how many orders fit. */
export function SlotDialog({ shopId, open, onClose }: { shopId: string; open: boolean; onClose: () => void }) {
  const create = useCreateSlots(shopId);
  const [date, setDate] = useState(() => addDays(colomboDay(0), 1));
  const [start, setStart] = useState("09:00");
  const [end, setEnd] = useState("11:00");
  const [cutoff, setCutoff] = useState("60");
  const [capacity, setCapacity] = useState("10");
  const cut = parseInt10(cutoff);
  const cap = parseInt10(capacity);
  const valid = TIME.test(start) && TIME.test(end) && start < end && cut !== null && cap !== null && cap <= 500;

  const close = () => { create.reset(); onClose(); };
  const submit = () => {
    if (!valid) return;
    const startsAt = colomboIso(date, start);
    const cutoffAt = new Date(new Date(startsAt).getTime() - cut * 60_000).toISOString();
    create.mutate([{ startsAt, endsAt: colomboIso(date, end), cutoffAt, capacity: cap }], { onSuccess: close });
  };

  return (
    <Modal open={open} onClose={close} title={t("admin.slots.new")}
      footer={<Button className="flex-1" loading={create.isPending} disabled={!valid} onClick={submit}>{t("admin.slots.create")}</Button>}>
      <div className="grid grid-cols-2 gap-3">
        <Field label={t("admin.slots.date")} className="col-span-2">{(p) => <Input {...p} type="date" min={colomboDay(0)} value={date} onChange={(e) => setDate(e.target.value)} />}</Field>
        <Field label={t("admin.slots.start")} error={fieldText(create.error, "slots")}>{(p) => <Input {...p} inputMode="numeric" placeholder="09:00" maxLength={5} value={start} onChange={(e) => setStart(e.target.value)} />}</Field>
        <Field label={t("admin.slots.end")}>{(p) => <Input {...p} inputMode="numeric" placeholder="11:00" maxLength={5} value={end} onChange={(e) => setEnd(e.target.value)} />}</Field>
        <NumberField label={t("admin.slots.cutoff")} hint={t("admin.slots.cutoffHint")} value={cutoff} onChange={setCutoff} />
        <NumberField label={t("admin.slots.capacity")} value={capacity} onChange={setCapacity} />
        <div className="col-span-2"><SaveNotice error={create.error} saved={false} /></div>
      </div>
    </Modal>
  );
}
