"use client";

import { useState } from "react";
import { Button, Checkbox, Field, Input, Modal, Select } from "@/components/ui";
import { couponStatusValues, couponTypeValues, fundedByValues } from "@/lib/enums.gen";
import { t } from "@/lib/i18n";
import { centsToRupeesText, parseRupees } from "@/lib/money";
import { colomboDay, fromColomboLocal, toColomboLocal } from "@/lib/time";
import { MoneyField, NumberField } from "../fields";
import { bpToText, fieldText, parseInt10, parsePercentBp, SaveNotice } from "../form";
import type { Coupon, Shop } from "../types";
import { useCreateCoupon, useUpdateCoupon } from "./api";

const CODE = /^[A-Za-z0-9]{4,20}$/;
const optMoney = (s: string) => (s.trim() === "" ? undefined : parseRupees(s));

interface Props {
  /** The coupon being changed, or null to make one. */
  coupon: Coupon | null;
  shops: Shop[];
  open: boolean;
  onClose: () => void;
}

/**
 * A coupon. When making one, everything can be set. Afterwards only the window, limits, minimum bill,
 * status and "allow loss" can change; the other boxes are shown but off.
 */
export function CouponDialog({ coupon, shops, open, onClose }: Props) {
  const create = useCreateCoupon();
  const update = useUpdateCoupon(coupon?.id ?? "");
  const mutation = coupon ? update : create;
  const [code, setCode] = useState(coupon?.code ?? "");
  const [shopId, setShopId] = useState(coupon?.shopId ?? "");
  const [type, setType] = useState<Coupon["type"]>(coupon?.type ?? "PERCENT");
  const [value, setValue] = useState(coupon ? (coupon.type === "PERCENT" ? bpToText(coupon.value) : centsToRupeesText(coupon.value)) : "");
  const [maxOff, setMaxOff] = useState(coupon?.maxDiscountCents != null ? centsToRupeesText(coupon.maxDiscountCents) : "");
  const [minOrder, setMinOrder] = useState(centsToRupeesText(coupon?.minOrderCents ?? 0));
  const [starts, setStarts] = useState(coupon ? toColomboLocal(coupon.startsAt) : `${colomboDay(0)}T00:00`);
  const [ends, setEnds] = useState(coupon ? toColomboLocal(coupon.endsAt) : "");
  const [limit, setLimit] = useState(coupon?.usageLimit != null ? String(coupon.usageLimit) : "");
  const [perUser, setPerUser] = useState(String(coupon?.perUserLimit ?? 1));
  const [fundedBy, setFundedBy] = useState<Coupon["fundedBy"]>(coupon?.fundedBy ?? "PLATFORM");
  const [allowLoss, setAllowLoss] = useState(coupon?.allowLoss ?? false);
  const [status, setStatus] = useState<Coupon["status"]>(coupon?.status ?? "ACTIVE");

  const amount = type === "PERCENT" ? parsePercentBp(value) : parseRupees(value);
  const maxOffCents = optMoney(maxOff);
  const minCents = parseRupees(minOrder);
  const startsAt = fromColomboLocal(starts);
  const endsAt = fromColomboLocal(ends);
  const usage = limit.trim() === "" ? null : parseInt10(limit);
  const per = parseInt10(perUser);
  const valid = (coupon !== null || (CODE.test(code) && amount !== null && amount > 0 && (fundedBy === "PLATFORM" || shopId !== "")))
    && maxOffCents !== null && minCents !== null && startsAt !== null && endsAt !== null && endsAt > startsAt
    && (limit.trim() === "" || usage !== null) && per !== null && per >= 1 && per <= 100;
  const close = () => { mutation.reset(); onClose(); };

  const submit = () => {
    if (!valid) return;
    const common = { startsAt, endsAt, usageLimit: usage, perUserLimit: per, maxDiscountCents: maxOffCents ?? null, minOrderCents: minCents, allowLoss: fundedBy === "PLATFORM" && allowLoss };
    if (coupon) update.mutate({ ...common, status }, { onSuccess: close });
    else create.mutate({ ...common, code, shopId: shopId || null, type, value: amount!, fundedBy }, { onSuccess: close });
  };
  const locked = coupon !== null;

  return (
    <Modal open={open} onClose={close} title={coupon ? t("admin.coupons.edit") : t("admin.coupons.new")}
      footer={<Button className="flex-1" loading={mutation.isPending} disabled={!valid} onClick={submit}>{t("common.save")}</Button>}>
      <div className="grid grid-cols-2 gap-3">
        <Field label={t("admin.coupons.code")} hint={t("admin.coupons.codeHint")} error={fieldText(mutation.error, "code")}>{(p) => <Input {...p} value={code} disabled={locked} maxLength={20} autoCapitalize="characters" onChange={(e) => setCode(e.target.value.toUpperCase())} />}</Field>
        <Field label={t("admin.coupons.shop")} error={fieldText(mutation.error, "shopId")}>
          {(p) => <Select {...p} value={shopId} disabled={locked} onChange={(e) => setShopId(e.target.value)}><option value="">{t("admin.issues.allShops")}</option>{shops.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}</Select>}
        </Field>
        <Field label={t("admin.coupons.type")}>{(p) => <Select {...p} value={type} disabled={locked} onChange={(e) => setType(e.target.value as Coupon["type"])}>{couponTypeValues.map((v) => <option key={v} value={v}>{t(`admin.coupons.type_${v}`)}</option>)}</Select>}</Field>
        <MoneyField label={type === "PERCENT" ? t("admin.coupons.percent") : t("admin.coupons.amount")} value={value} onChange={setValue} error={fieldText(mutation.error, "value")} />
        <MoneyField label={t("admin.coupons.maxOff")} hint={t("admin.fees.blank")} value={maxOff} onChange={setMaxOff} />
        <MoneyField label={t("admin.coupons.minOrder")} value={minOrder} onChange={setMinOrder} />
        <Field label={t("admin.coupons.starts")} error={fieldText(mutation.error, "startsAt")}>{(p) => <Input {...p} type="datetime-local" value={starts} onChange={(e) => setStarts(e.target.value)} />}</Field>
        <Field label={t("admin.coupons.ends")} error={fieldText(mutation.error, "endsAt")}>{(p) => <Input {...p} type="datetime-local" value={ends} onChange={(e) => setEnds(e.target.value)} />}</Field>
        <NumberField label={t("admin.coupons.usageLimit")} hint={t("admin.fees.blank")} value={limit} onChange={setLimit} error={fieldText(mutation.error, "usageLimit")} />
        <NumberField label={t("admin.coupons.perUser")} value={perUser} onChange={setPerUser} error={fieldText(mutation.error, "perUserLimit")} />
        <Field label={t("admin.coupons.fundedBy")} error={fieldText(mutation.error, "fundedBy")}>{(p) => <Select {...p} value={fundedBy} disabled={locked} onChange={(e) => setFundedBy(e.target.value as Coupon["fundedBy"])}>{fundedByValues.map((v) => <option key={v} value={v}>{t(`admin.coupons.funded_${v}`)}</option>)}</Select>}</Field>
        {coupon && <Field label={t("admin.shops.status")}>{(p) => <Select {...p} value={status} onChange={(e) => setStatus(e.target.value as Coupon["status"])}>{couponStatusValues.map((v) => <option key={v} value={v}>{t(`admin.coupons.status_${v}`)}</option>)}</Select>}</Field>}
        <Checkbox label={t("admin.coupons.allowLoss")} checked={allowLoss} disabled={fundedBy === "SHOP"} onChange={setAllowLoss} className="col-span-2" />
        <div className="col-span-2"><SaveNotice error={mutation.error} saved={false} /></div>
      </div>
    </Modal>
  );
}
