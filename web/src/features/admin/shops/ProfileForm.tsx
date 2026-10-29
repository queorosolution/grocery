"use client";

import { useState, type ReactNode } from "react";
import { Button, Field, Input, MapPicker } from "@/components/ui";
import { inSriLanka, type LatLng } from "@/lib/geo";
import { t } from "@/lib/i18n";
import { fieldText, SaveNotice } from "../form";
import type { Shop } from "../types";
import type { ProfileBody } from "./shopApi";

const SLUG = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const COLOUR = /^#[0-9a-fA-F]{6}$/;

interface Props {
  initial?: Shop;
  submitLabel: string;
  pending: boolean;
  error: unknown;
  saved?: boolean;
  onSubmit: (body: ProfileBody) => void;
  /** Extra fields under the common ones (the edit page adds the shop status). */
  extra?: ReactNode;
}

/** The shop profile, for creating a shop and for editing one. A change of slug, name or logo rebuilds the site (D30). */
export function ProfileForm({ initial, submitLabel, pending, error, saved = false, onSubmit, extra }: Props) {
  const [name, setName] = useState(initial?.name ?? "");
  const [slug, setSlug] = useState(initial?.slug ?? "");
  const [phone, setPhone] = useState(initial?.phone ?? "");
  const [address, setAddress] = useState(initial?.addressText ?? "");
  const [pin, setPin] = useState<LatLng | null>(initial ? { lat: initial.lat, lng: initial.lng } : null);
  const [logo, setLogo] = useState(initial?.logoUrl ?? "");
  const [colour, setColour] = useState(initial?.themeColor ?? "#1a7f4b");
  const valid = name.trim().length >= 2 && SLUG.test(slug) && COLOUR.test(colour) && !!pin && inSriLanka(pin);

  const submit = () => {
    if (!pin) return;
    onSubmit({
      name: name.trim(), slug, phone: phone.trim() || null, addressText: address.trim() || null,
      lat: pin.lat, lng: pin.lng, logoUrl: logo.trim() || null, themeColor: colour,
    });
  };

  return (
    <form className="flex flex-col gap-3" onSubmit={(e) => { e.preventDefault(); if (valid) submit(); }}>
      <Field label={t("admin.shops.name")} error={fieldText(error, "name")}>{(p) => <Input {...p} value={name} maxLength={80} onChange={(e) => setName(e.target.value)} />}</Field>
      <Field label={t("admin.shops.slug")} hint={t("admin.shops.slugHint")} error={fieldText(error, "slug")}>
        {(p) => <Input {...p} value={slug} maxLength={60} autoCapitalize="none" onChange={(e) => setSlug(e.target.value.toLowerCase())} />}
      </Field>
      <Field label={t("admin.shops.phone")} error={fieldText(error, "phone")}>{(p) => <Input {...p} type="tel" value={phone} onChange={(e) => setPhone(e.target.value)} />}</Field>
      <Field label={t("admin.shops.address")} error={fieldText(error, "addressText")}>{(p) => <Input {...p} value={address} maxLength={200} onChange={(e) => setAddress(e.target.value)} />}</Field>
      <Field label={t("admin.shops.pin")}>{() => <MapPicker value={pin} onChange={setPin} error={pin && !inSriLanka(pin) ? t("admin.field.invalid") : fieldText(error, "lat")} />}</Field>
      <Field label={t("admin.shop.profile.logo")} hint={t("admin.shop.profile.logoHint")} error={fieldText(error, "logoUrl")}>
        {(p) => <Input {...p} type="url" value={logo} onChange={(e) => setLogo(e.target.value)} />}
      </Field>
      <Field label={t("admin.shops.theme")} error={fieldText(error, "themeColor")}>
        {(p) => <Input {...p} value={colour} maxLength={7} onChange={(e) => setColour(e.target.value)} />}
      </Field>
      {extra}
      <SaveNotice error={error} saved={saved} />
      <Button type="submit" loading={pending} disabled={!valid} className="self-start">{submitLabel}</Button>
    </form>
  );
}

