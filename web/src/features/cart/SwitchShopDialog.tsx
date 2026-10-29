"use client";

import { ConfirmDialog } from "@/components/ui";
import { t } from "@/lib/i18n";

interface Props {
  from: string;
  to: string;
  open: boolean;
  onConfirm: () => void;
  onClose: () => void;
}

/** One cart holds one shop. Adding from another shop first asks to start a new cart. */
export function SwitchShopDialog({ from, to, open, onConfirm, onClose }: Props) {
  return (
    <ConfirmDialog
      open={open}
      title={t("cart.switchTitle")}
      message={t("cart.switchBody", { from, to })}
      confirmLabel={t("cart.switchConfirm")}
      onConfirm={onConfirm}
      onClose={onClose}
    />
  );
}
