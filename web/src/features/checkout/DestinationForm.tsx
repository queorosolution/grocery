"use client";

import { useState } from "react";
import { Field, Input, MapPicker } from "@/components/ui";
import { inSriLanka, type LatLng } from "@/lib/geo";
import { t } from "@/lib/i18n";
import type { Destination } from "./types";

interface Props {
  /** The destination when it is complete and inside Sri Lanka, else null. */
  onChange: (d: Destination | null) => void;
}

const MIN_LINE = 3; // the same limits as a saved address (docs/06 section 5)

/** "Deliver to another place": a line, a landmark and a map pin, for this order only. */
export function DestinationForm({ onChange }: Props) {
  const [label, setLabel] = useState("");
  const [line1, setLine1] = useState("");
  const [landmark, setLandmark] = useState("");
  const [pin, setPin] = useState<LatLng | null>(null);

  // Told to the parent from the handlers, with the value that just changed.
  const tell = (next: Partial<{ label: string; line1: string; landmark: string; pin: LatLng | null }>) => {
    const v = { label, line1, landmark, pin, ...next };
    const ok = v.pin !== null && inSriLanka(v.pin) && v.line1.trim().length >= MIN_LINE;
    onChange(ok && v.pin ? {
      line1: v.line1.trim(), lat: v.pin.lat, lng: v.pin.lng,
      ...(v.label.trim() ? { label: v.label.trim() } : {}),
      ...(v.landmark.trim() ? { landmark: v.landmark.trim() } : {}),
    } : null);
  };

  return (
    <div className="mt-3 flex flex-col gap-3">
      <Field label={t("checkout.otherLine")} hint={t("address.line1Hint")}>
        {(p) => <Input {...p} maxLength={160} autoComplete="off" value={line1} onChange={(e) => { setLine1(e.target.value); tell({ line1: e.target.value }); }} />}
      </Field>
      <Field label={t("checkout.otherLandmark")}>
        {(p) => <Input {...p} maxLength={120} value={landmark} onChange={(e) => { setLandmark(e.target.value); tell({ landmark: e.target.value }); }} />}
      </Field>
      <Field label={t("checkout.otherLabel")} hint={t("checkout.otherLabelHint")}>
        {(p) => <Input {...p} maxLength={30} value={label} onChange={(e) => { setLabel(e.target.value); tell({ label: e.target.value }); }} />}
      </Field>
      <div>
        <p className="mb-1 text-sm font-semibold">{t("address.pin")}</p>
        <MapPicker value={pin} onChange={(p) => { setPin(p); tell({ pin: p }); }}
          error={pin && !inSriLanka(pin) ? t("address.outside") : undefined} />
      </div>
      <p className="text-sm text-muted">{t("checkout.otherNote")}</p>
    </div>
  );
}
