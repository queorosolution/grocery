"use client";

import { t } from "@/lib/i18n";
import { Button } from "./Button";
import { Dialog, type DialogProps } from "./Dialog";

export function Modal(props: DialogProps) {
  return <Dialog shape="modal" {...props} />;
}

interface ConfirmDialogProps {
  open: boolean;
  title: string;
  message: string;
  confirmLabel?: string;
  /** Use for actions that cannot be undone. */
  danger?: boolean;
  loading?: boolean;
  onConfirm: () => void;
  onClose: () => void;
}

export function ConfirmDialog({ open, title, message, confirmLabel, danger, loading, onConfirm, onClose }: ConfirmDialogProps) {
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={title}
      footer={
        <>
          <Button variant="secondary" className="flex-1" onClick={onClose}>{t("common.cancel")}</Button>
          <Button variant={danger ? "danger" : "primary"} className="flex-1" loading={loading} onClick={onConfirm}>
            {confirmLabel ?? t("common.confirm")}
          </Button>
        </>
      }
    >
      <p>{message}</p>
    </Modal>
  );
}
