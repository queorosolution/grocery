"use client";

import { Field, Select } from "@/components/ui";
import { shopStatusValues } from "@/lib/enums.gen";
import { t } from "@/lib/i18n";
import { fieldText } from "../form";
import type { ShopDetail } from "../types";
import { ProfileForm } from "./ProfileForm";
import { statusText } from "./ShopsScreen";
import { useUpdateShop } from "./shopApi";
import { useState } from "react";

export function ProfileTab({ detail }: { detail: ShopDetail }) {
  const { shop } = detail;
  const update = useUpdateShop(shop.id);
  const [status, setStatus] = useState(shop.status);
  return (
    <ProfileForm initial={shop} submitLabel={t("admin.shop.profile.save")} pending={update.isPending} error={update.error} saved={update.isSuccess}
      onSubmit={(body) => update.mutate({ ...body, status })}
      extra={(
        <Field label={t("admin.shops.status")} error={fieldText(update.error, "status")}>
          {(p) => <Select {...p} value={status} onChange={(e) => setStatus(e.target.value as ShopDetail["shop"]["status"])}>{shopStatusValues.map((s) => <option key={s} value={s}>{t(statusText[s])}</option>)}</Select>}
        </Field>
      )} />
  );
}
