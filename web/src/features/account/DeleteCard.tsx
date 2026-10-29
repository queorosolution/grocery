"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { Banner, Button, Card, Field, Input, Modal } from "@/components/ui";
import { useDeleteAccount } from "@/features/auth/api";
import { ApiError } from "@/lib/api";
import { errorText } from "@/lib/errorText";
import { t, type MessageKey } from "@/lib/i18n";
import { useCartReasons } from "./useCartReasons";

// The API names what is open in `fields`; these are the words for each.
const reasonText: Record<string, MessageKey> = {
  orders: "account.blocked.orders",
  refunds: "account.blocked.refunds",
  team: "account.blocked.team",
};

/** Delete account: a soft delete the API refuses while anything is open (docs/06 section 5). */
export function DeleteCard() {
  const router = useRouter();
  const remove = useDeleteAccount();
  const cart = useCartReasons();
  const [open, setOpen] = useState(false);
  const [word, setWord] = useState("");

  const close = () => {
    setOpen(false);
    setWord("");
    remove.reset();
  };
  const submit = (e: FormEvent) => {
    e.preventDefault();
    remove.mutate(undefined, { onSuccess: () => router.replace("/") });
  };

  const refused = remove.error instanceof ApiError && remove.error.code === "ACCOUNT_NOT_DELETABLE";
  const reasons: MessageKey[] = refused
    ? Object.keys((remove.error as ApiError).fields).flatMap((k) => (reasonText[k] ? [reasonText[k]] : []))
    : [];
  const blocked = [...cart, ...reasons];

  return (
    <Card className="p-4">
      <Button variant="ghost" className="text-danger" onClick={() => setOpen(true)}>{t("account.delete")}</Button>
      <Modal open={open} onClose={close} title={t("account.deleteTitle")}>
        {cart.length > 0 ? (
          <Reasons keys={cart} />
        ) : (
          <form onSubmit={submit} className="flex flex-col gap-3">
            <p className="text-muted">{t("account.deleteBody")}</p>
            {blocked.length > 0 && <Reasons keys={blocked} />}
            {remove.error && !refused && <Banner tone="danger">{errorText(remove.error)}</Banner>}
            <Field label={t("account.deleteType")}>
              {(p) => <Input {...p} autoComplete="off" value={word} onChange={(e) => setWord(e.target.value)} />}
            </Field>
            <div className="flex gap-2">
              <Button variant="secondary" className="flex-1" onClick={close}>{t("common.cancel")}</Button>
              <Button type="submit" variant="danger" className="flex-1" loading={remove.isPending} disabled={word !== "DELETE"}>
                {t("account.deleteConfirm")}
              </Button>
            </div>
          </form>
        )}
      </Modal>
    </Card>
  );
}

function Reasons({ keys }: { keys: readonly MessageKey[] }) {
  return (
    <Banner tone="warning">
      <p className="font-semibold">{t("account.deleteBlocked")}</p>
      <ul className="mt-1 list-disc pl-5">
        {keys.map((k) => <li key={k}>{t(k)}</li>)}
      </ul>
    </Banner>
  );
}
