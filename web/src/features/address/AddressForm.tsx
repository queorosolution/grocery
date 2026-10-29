"use client";

import { useState, type FormEvent } from "react";
import { Button, Checkbox, Chips, Field, Input, MapPicker } from "@/components/ui";
import { errorText, fieldErrors } from "@/lib/errorText";
import { inSriLanka, type LatLng } from "@/lib/geo";
import { t } from "@/lib/i18n";
import type { Address, AddressInput } from "./types";

const PRESETS = [t("address.home"), t("address.work")].map((label) => ({ id: label, label }));

// Server field codes → sentences. Anything not listed shows the API's own message.
const serverTexts: Record<string, string> = {
  "lat.out_of_range": t("address.outside"),
  "lng.out_of_range": t("address.outside"),
  addresses: t("address.tooMany"),
};

interface AddressFormProps {
  initial?: Address;
  /** The first address is the default, and the default cannot be switched off, only moved. */
  lockDefault: boolean;
  submitLabel: string;
  loading: boolean;
  error: unknown;
  onSubmit: (input: AddressInput) => void;
  onCancel?: () => void;
}

export function AddressForm({ initial, lockDefault, submitLabel, loading, error, onSubmit, onCancel }: AddressFormProps) {
  const [label, setLabel] = useState(initial?.label ?? t("address.home"));
  const [line1, setLine1] = useState(initial?.line1 ?? "");
  const [landmark, setLandmark] = useState(initial?.landmark ?? "");
  const [pin, setPin] = useState<LatLng | null>(initial ? { lat: initial.lat, lng: initial.lng } : null);
  const [isDefault, setIsDefault] = useState(initial?.isDefault ?? lockDefault);
  const [tried, setTried] = useState(false);

  const mine: Record<string, string> = {};
  if (label.trim().length < 1) mine.label = t("address.labelMissing");
  if (line1.trim().length < 3) mine.line1 = t("address.lineShort");
  if (!pin) mine.pin = t("address.pinMissing");
  else if (!inSriLanka(pin)) mine.pin = t("address.outside");
  const server = fieldErrors(error, serverTexts);
  // Reasons that belong to no single box (too many addresses, the main address cannot be switched off).
  const named = ["label", "line1", "landmark", "lat", "lng"];
  const general = error == null ? undefined : (Object.entries(server).find(([k]) => !named.includes(k))?.[1] ?? (Object.keys(server).length === 0 ? errorText(error) : undefined));
  const shown = (name: string, own = name) => (tried ? mine[own] : undefined) ?? server[name];

  const submit = (e: FormEvent) => {
    e.preventDefault();
    setTried(true);
    if (Object.keys(mine).length > 0 || !pin) return;
    onSubmit({ label, line1, landmark, lat: pin.lat, lng: pin.lng, isDefault: lockDefault || isDefault });
  };

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-3">
      <Field label={t("address.label")} error={shown("label")}>
        {(p) => (
          <>
            <Input {...p} maxLength={30} value={label} onChange={(e) => setLabel(e.target.value)} />
            <Chips label={t("address.presets")} items={PRESETS} value={label} onChange={(id) => id && setLabel(id)} className="mt-1" />
          </>
        )}
      </Field>
      <Field label={t("address.line1")} hint={t("address.line1Hint")} error={shown("line1")}>
        {(p) => <Input {...p} maxLength={160} autoComplete="street-address" value={line1} onChange={(e) => setLine1(e.target.value)} />}
      </Field>
      <Field label={t("address.landmark")} error={shown("landmark")}>
        {(p) => <Input {...p} maxLength={120} value={landmark} onChange={(e) => setLandmark(e.target.value)} />}
      </Field>
      <div>
        <p className="mb-1 text-sm font-semibold">{t("address.pin")}</p>
        <MapPicker value={pin} onChange={setPin} error={shown("lat", "pin") ?? server.lng} />
      </div>
      {!lockDefault && !initial?.isDefault && <Checkbox label={t("address.makeDefault")} checked={isDefault} onChange={setIsDefault} />}
      {general && <p role="alert" className="text-sm text-danger">{general}</p>}
      <div className="flex gap-2">
        {onCancel && <Button variant="secondary" className="flex-1" onClick={onCancel}>{t("common.cancel")}</Button>}
        <Button type="submit" className="flex-1" loading={loading}>{submitLabel}</Button>
      </div>
    </form>
  );
}
