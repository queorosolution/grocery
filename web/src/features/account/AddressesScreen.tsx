"use client";

import { useState } from "react";
import { Button, ConfirmDialog, EmptyState, Sheet, Skeleton, useToast } from "@/components/ui";
import { useAddAddress, useAddresses, useDeleteAddress, useUpdateAddress } from "@/features/address/api";
import { AddressForm } from "@/features/address/AddressForm";
import { DeliveryNote } from "@/features/address/DeliveryNote";
import { MAX_ADDRESSES, type Address, type AddressResult } from "@/features/address/types";
import { RequireUser } from "@/features/auth/guards";
import { Page } from "@/features/shell/Page";
import { t } from "@/lib/i18n";
import { AddressCard } from "./AddressCard";

/** "new", an address being edited, or nothing. */
type Editing = "new" | Address | null;

function List() {
  const q = useAddresses();
  const add = useAddAddress();
  const update = useUpdateAddress();
  const remove = useDeleteAddress();
  const toast = useToast();
  const [editing, setEditing] = useState<Editing>(null);
  const [removing, setRemoving] = useState<Address | null>(null);
  const [result, setResult] = useState<AddressResult | null>(null);

  const close = () => {
    setEditing(null);
    setResult(null);
    add.reset();
    update.reset();
  };
  const saved = { onSuccess: (r: AddressResult) => setResult(r) };
  const list = q.data ?? [];
  const mutation = editing === "new" ? add : update;

  if (q.isPending) return <Skeleton className="h-28 w-full" />;
  if (q.isError) return <EmptyState title={t("guard.failedTitle")} action={<Button onClick={() => void q.refetch()}>{t("common.retry")}</Button>} />;
  return (
    <div className="flex flex-col gap-3">
      {list.length === 0 && <EmptyState title={t("address.emptyTitle")} body={t("address.emptyBody")} />}
      {list.map((a) => (
        <AddressCard key={a.id} address={a} busy={update.isPending && update.variables?.id === a.id} onEdit={() => setEditing(a)} onDelete={() => setRemoving(a)}
          onMakeDefault={() => update.mutate({ id: a.id, input: { label: a.label, line1: a.line1, landmark: a.landmark ?? "", lat: a.lat, lng: a.lng, isDefault: true } })} />
      ))}
      <Button disabled={list.length >= MAX_ADDRESSES} onClick={() => setEditing("new")}>{t("address.add")}</Button>
      {list.length >= MAX_ADDRESSES && <p className="text-sm text-muted">{t("address.tooMany")}</p>}

      <Sheet open={editing !== null} onClose={close} title={editing === "new" ? t("address.add") : t("address.edit")}>
        {result ? (
          <div className="flex flex-col gap-3">
            <DeliveryNote shops={result.deliveringShops} />
            <Button onClick={close}>{t("common.done")}</Button>
          </div>
        ) : editing ? (
          <AddressForm key={editing === "new" ? "new" : editing.id} initial={editing === "new" ? undefined : editing} lockDefault={list.length === 0}
            submitLabel={t("address.save")} loading={mutation.isPending} error={mutation.error} onCancel={close}
            onSubmit={(input) => (editing === "new" ? add.mutate(input, saved) : update.mutate({ id: editing.id, input }, saved))} />
        ) : null}
      </Sheet>

      <ConfirmDialog open={removing !== null} danger title={t("address.removeTitle")} message={t("address.removeBody", { label: removing?.label ?? "" })}
        confirmLabel={t("address.remove")} loading={remove.isPending} onClose={() => setRemoving(null)}
        onConfirm={() => removing && remove.mutate(removing.id, { onSuccess: () => { setRemoving(null); toast.show(t("address.removed")); } })} />
    </div>
  );
}

export function AddressesScreen() {
  return (
    <Page className="max-w-xl">
      <h1 className="mb-4 text-xl font-semibold md:text-2xl">{t("account.addresses")}</h1>
      <RequireUser>
        <List />
      </RequireUser>
    </Page>
  );
}
