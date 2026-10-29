"use client";

import { Banner, Button, Card } from "@/components/ui";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import type { ShopDetail } from "../types";
import { useCartSync, useCreateCartTenant } from "./shopApi";

/**
 * The shop's tenant in the cart service. Only the public key is ever shown: the secret key is
 * not kept anywhere (docs/06 section 10).
 */
export function CartCard({ detail }: { detail: ShopDetail }) {
  const { settings, shop } = detail;
  const create = useCreateCartTenant(shop.id);
  const sync = useCartSync(shop.id);
  const error = create.error ?? sync.error;
  return (
    <Card className="flex flex-col gap-2 p-4">
      <h2 className="text-lg font-semibold">{t("admin.cart.title")}</h2>
      {settings.cartTenantId ? (
        <>
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
            <dt className="text-muted">{t("admin.cart.tenant")}</dt><dd className="break-all">{settings.cartTenantId}</dd>
            <dt className="text-muted">{t("admin.cart.publicKey")}</dt><dd className="break-all">{settings.cartPublicKey}</dd>
          </dl>
          <Button variant="secondary" className="self-start" loading={sync.isPending} onClick={() => sync.mutate(undefined)}>{t("admin.cart.sync")}</Button>
          {sync.data && <p role="status" className="text-sm text-success">{t("admin.cart.queued", { count: sync.data.queuedItems })}</p>}
        </>
      ) : (
        <>
          <p className="text-muted">{t("admin.cart.off")}</p>
          <Button className="self-start" loading={create.isPending} onClick={() => create.mutate(undefined)}>{t("admin.cart.enable")}</Button>
        </>
      )}
      {error && <Banner tone="danger">{errorText(error)}</Banner>}
    </Card>
  );
}
