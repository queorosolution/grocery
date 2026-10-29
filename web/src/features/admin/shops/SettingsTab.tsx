"use client";

import { useState } from "react";
import { Button, Checkbox, Field, Select } from "@/components/ui";
import { deliveryTypeValues, driverSourceValues, notifyChannelValues, paymentProviderValues, type NotifyChannel } from "@/lib/enums.gen";
import { t } from "@/lib/i18n";
import { centsToRupeesText, parseRupees } from "@/lib/money";
import { MoneyField, NumberField } from "../fields";
import { fieldText, parseInt10, SaveNotice } from "../form";
import type { Settings, ShopDetail } from "../types";
import { CartCard } from "./CartCard";
import { usePaymentAccounts } from "./paymentApi";
import { useSaveSettings } from "./shopApi";

const NUMBERS = ["markupBp", "maxLines", "reviewTimeoutMin", "revisionTimeoutMin", "paymentTimeoutMin"] as const;

/** The form's state: every number is text until it is saved. */
type Draft = Pick<Settings, "deliveryType" | "paymentProvider" | "paymentAccountId" | "driverSource" | "notifyChannels">
  & Record<(typeof NUMBERS)[number] | "minOrder" | "minMargin", string>;

/** Per-shop options (docs/05 shop_settings). Money is typed in rupees and sent as exact cents. */
export function SettingsTab({ detail }: { detail: ShopDetail }) {
  const { settings, shop } = detail;
  const save = useSaveSettings(shop.id);
  const accounts = usePaymentAccounts();
  const [s, setS] = useState<Draft>(() => ({
    deliveryType: settings.deliveryType, paymentProvider: settings.paymentProvider, paymentAccountId: settings.paymentAccountId,
    driverSource: settings.driverSource, notifyChannels: settings.notifyChannels,
    minOrder: centsToRupeesText(settings.minOrderCents),
    minMargin: centsToRupeesText(settings.minPlatformMarginCents),
    markupBp: String(settings.markupBp), maxLines: String(settings.maxLines), reviewTimeoutMin: String(settings.reviewTimeoutMin),
    revisionTimeoutMin: String(settings.revisionTimeoutMin), paymentTimeoutMin: String(settings.paymentTimeoutMin),
  }));
  const set = <K extends keyof Draft>(k: K, v: Draft[K]) => setS((o) => ({ ...o, [k]: v }));
  const usable = (accounts.data?.data ?? []).filter((a) => a.ownerType === "PLATFORM" || a.shopId === shop.id);

  const nums = Object.fromEntries(NUMBERS.map((k) => [k, parseInt10(s[k])]));
  const minOrder = parseRupees(s.minOrder);
  const minMargin = parseRupees(s.minMargin);
  const valid = minOrder !== null && minMargin !== null && Object.values(nums).every((v) => v !== null);
  const submit = () => {
    if (!valid) return;
    const body: Settings = {
      deliveryType: s.deliveryType, paymentProvider: s.paymentProvider, paymentAccountId: s.paymentAccountId, driverSource: s.driverSource,
      notifyChannels: s.notifyChannels, minOrderCents: minOrder, minPlatformMarginCents: minMargin,
      markupBp: nums.markupBp!, maxLines: nums.maxLines!, reviewTimeoutMin: nums.reviewTimeoutMin!,
      revisionTimeoutMin: nums.revisionTimeoutMin!, paymentTimeoutMin: nums.paymentTimeoutMin!,
    };
    save.mutate(body);
  };
  const toggle = (c: NotifyChannel, on: boolean) => set("notifyChannels", on ? [...s.notifyChannels, c] : s.notifyChannels.filter((x) => x !== c));

  return (
    <div className="flex flex-col gap-5">
      <form className="grid gap-3 md:grid-cols-2" onSubmit={(e) => { e.preventDefault(); submit(); }}>
        <Field label={t("admin.settings.deliveryType")}>{(p) => <Select {...p} value={s.deliveryType} onChange={(e) => set("deliveryType", e.target.value as Settings["deliveryType"])}>{deliveryTypeValues.map((v) => <option key={v} value={v}>{t(`admin.settings.deliveryType_${v}`)}</option>)}</Select>}</Field>
        <Field label={t("admin.settings.driverSource")}>{(p) => <Select {...p} value={s.driverSource} onChange={(e) => set("driverSource", e.target.value as Settings["driverSource"])}>{driverSourceValues.map((v) => <option key={v} value={v}>{t(`admin.settings.driverSource_${v}`)}</option>)}</Select>}</Field>
        <Field label={t("admin.settings.paymentProvider")}>{(p) => <Select {...p} value={s.paymentProvider} onChange={(e) => set("paymentProvider", e.target.value as Settings["paymentProvider"])}>{paymentProviderValues.map((v) => <option key={v} value={v}>{t(`admin.payment.provider_${v}`)}</option>)}</Select>}</Field>
        <Field label={t("admin.settings.paymentAccount")} error={fieldText(save.error, "paymentAccountId")}>
          {(p) => (
            <Select {...p} value={s.paymentAccountId ?? ""} onChange={(e) => set("paymentAccountId", e.target.value || null)}>
              <option value="">{t("admin.settings.noAccount")}</option>
              {usable.map((a) => <option key={a.id} value={a.id}>{t(`admin.payment.provider_${a.provider}`)} · {a.merchantId ?? a.bank?.bankName ?? a.id.slice(0, 8)}</option>)}
            </Select>
          )}
        </Field>
        <fieldset className="md:col-span-2">
          <legend className="text-sm font-semibold">{t("admin.settings.channels")}</legend>
          <div className="flex flex-wrap gap-x-6">{notifyChannelValues.map((c) => <Checkbox key={c} label={t(`admin.settings.channel_${c}`)} checked={s.notifyChannels.includes(c)} onChange={(on) => toggle(c, on)} />)}</div>
        </fieldset>
        <NumberField label={t("admin.settings.markupBp")} hint={t("admin.settings.markupHint")} value={s.markupBp} onChange={(v) => set("markupBp", v)} error={fieldText(save.error, "markupBp")} />
        <MoneyField label={t("admin.settings.minMargin")} value={s.minMargin} onChange={(v) => set("minMargin", v)} error={fieldText(save.error, "minPlatformMarginCents")} />
        <MoneyField label={t("admin.settings.minOrder")} value={s.minOrder} onChange={(v) => set("minOrder", v)} error={fieldText(save.error, "minOrderCents")} />
        <NumberField label={t("admin.settings.maxLines")} value={s.maxLines} onChange={(v) => set("maxLines", v)} error={fieldText(save.error, "maxLines")} />
        <NumberField label={t("admin.settings.reviewTimeout")} hint={t("admin.settings.minutes")} value={s.reviewTimeoutMin} onChange={(v) => set("reviewTimeoutMin", v)} error={fieldText(save.error, "reviewTimeoutMin")} />
        <NumberField label={t("admin.settings.revisionTimeout")} hint={t("admin.settings.minutes")} value={s.revisionTimeoutMin} onChange={(v) => set("revisionTimeoutMin", v)} error={fieldText(save.error, "revisionTimeoutMin")} />
        <NumberField label={t("admin.settings.paymentTimeout")} hint={t("admin.settings.minutes")} value={s.paymentTimeoutMin} onChange={(v) => set("paymentTimeoutMin", v)} error={fieldText(save.error, "paymentTimeoutMin")} />
        <div className="flex flex-col gap-2 md:col-span-2">
          <SaveNotice error={save.error} saved={save.isSuccess} />
          <Button type="submit" loading={save.isPending} disabled={!valid} className="self-start">{t("admin.settings.save")}</Button>
        </div>
      </form>
      <CartCard detail={detail} />
    </div>
  );
}
