"use client";

import { useState, type FormEvent } from "react";
import { Banner, Button, Card, Field, Input } from "@/components/ui";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import { useApplyCoupon, useRemoveCoupon } from "./api";
import type { OrderDetail } from "./types";

/** Enter a coupon before paying. Shown from `allowedActions`: APPLY_COUPON to add, REMOVE_COUPON to take off. */
export function CouponBox({ order }: { order: OrderDetail }) {
  const apply = useApplyCoupon(order.id);
  const remove = useRemoveCoupon(order.id);
  const [code, setCode] = useState("");
  const canApply = order.allowedActions.includes("APPLY_COUPON");
  const canRemove = order.allowedActions.includes("REMOVE_COUPON");

  if (order.coupon) {
    return (
      <Banner tone="success">
        <div className="flex items-center justify-between gap-2">
          <span>{t("order.couponApplied", { code: order.coupon.code, amount: formatLKR(order.coupon.discountCents) })}</span>
          {canRemove && <Button variant="ghost" loading={remove.isPending} onClick={() => remove.mutate(order.version)}>{t("order.couponRemove")}</Button>}
        </div>
        {remove.error && <p role="alert" className="mt-1 text-sm text-danger">{errorText(remove.error)}</p>}
      </Banner>
    );
  }
  if (!canApply) return null;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (code.trim()) apply.mutate({ version: order.version, code: code.trim() }, { onSuccess: () => setCode("") });
  };
  return (
    <Card className="p-4">
      <form onSubmit={submit} className="flex items-end gap-2">
        <div className="flex-1">
          <Field label={t("order.couponTitle")} error={apply.error ? errorText(apply.error) : undefined}>
            {(p) => <Input {...p} aria-label={t("order.couponLabel")} placeholder={t("order.couponLabel")} autoCapitalize="characters" maxLength={30}
              value={code} onChange={(e) => setCode(e.target.value)} />}
          </Field>
        </div>
        <Button type="submit" variant="secondary" loading={apply.isPending} disabled={!code.trim()}>{t("order.couponApply")}</Button>
      </form>
    </Card>
  );
}
