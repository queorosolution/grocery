"use client";

import { useRouter } from "next/navigation";
import { Modal } from "@/components/ui";
import { t } from "@/lib/i18n";
import { ProfileForm } from "./ProfileForm";
import { useCreateShop } from "./shopApi";

/** Creating a shop saves it as a draft and opens its setup page. The site rebuild starts on the server. */
export function NewShopDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const create = useCreateShop();
  const router = useRouter();
  const close = () => { create.reset(); onClose(); };
  return (
    <Modal open={open} onClose={close} title={t("admin.shops.createTitle")}>
      <ProfileForm submitLabel={t("admin.shops.create")} pending={create.isPending} error={create.error}
        onSubmit={(body) => create.mutate(body, { onSuccess: (shop) => router.push(`/console/admin/shop/${shop.id}?tab=settings`) })} />
    </Modal>
  );
}
