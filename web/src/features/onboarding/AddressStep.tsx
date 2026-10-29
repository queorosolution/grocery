"use client";

import { Button } from "@/components/ui";
import { useAddAddress } from "@/features/address/api";
import { AddressForm } from "@/features/address/AddressForm";
import { DeliveryNote } from "@/features/address/DeliveryNote";
import { t } from "@/lib/i18n";

/** The first address is the default. After saving, show which shops deliver there before moving on. */
export function AddressStep({ onDone }: { onDone: () => void }) {
  const add = useAddAddress();
  if (add.data) {
    return (
      <div className="flex flex-col gap-3">
        <DeliveryNote shops={add.data.deliveringShops} />
        <Button onClick={onDone}>{t("onboarding.finish")}</Button>
      </div>
    );
  }
  return (
    <div className="flex flex-col gap-3">
      <AddressForm lockDefault submitLabel={t("address.save")} loading={add.isPending} error={add.error} onSubmit={(input) => add.mutate(input)} />
      <Button variant="ghost" onClick={onDone}>{t("onboarding.skip")}</Button>
    </div>
  );
}
