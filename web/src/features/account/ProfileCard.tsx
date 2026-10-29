"use client";

import { useState, type FormEvent, type ReactNode } from "react";
import { Badge, Button, Card, Field, Input, Modal, useToast } from "@/components/ui";
import { useUpdateName } from "@/features/auth/api";
import { PhoneVerify } from "@/features/auth/PhoneVerify";
import type { Me } from "@/features/auth/types";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";

function Row({ label, action, children }: { label: string; action?: ReactNode; children: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-line py-3 last:border-b-0">
      <div className="min-w-0">
        <p className="text-sm text-muted">{label}</p>
        <div className="truncate">{children}</div>
      </div>
      {action}
    </div>
  );
}

function NameForm({ me, onDone }: { me: Me; onDone: () => void }) {
  const update = useUpdateName();
  const toast = useToast();
  const [name, setName] = useState(me.name);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    update.mutate(name.trim(), { onSuccess: () => { toast.show(t("account.saved")); onDone(); } });
  };
  return (
    <form onSubmit={submit} className="flex flex-col gap-3 py-3">
      <Field label={t("account.name")} error={update.error ? errorText(update.error) : undefined}>
        {(p) => <Input {...p} maxLength={80} autoComplete="name" value={name} onChange={(e) => setName(e.target.value)} />}
      </Field>
      <div className="flex gap-2">
        <Button variant="secondary" className="flex-1" onClick={onDone}>{t("common.cancel")}</Button>
        <Button type="submit" className="flex-1" loading={update.isPending} disabled={name.trim().length < 1}>{t("common.save")}</Button>
      </div>
    </form>
  );
}

/** Name (editable), e-mail (fixed: it is the Google account) and phone (changed by a new SMS code). */
export function ProfileCard({ me }: { me: Me }) {
  const toast = useToast();
  const [editName, setEditName] = useState(false);
  const [editPhone, setEditPhone] = useState(false);
  return (
    <Card className="px-4">
      {editName ? (
        <NameForm me={me} onDone={() => setEditName(false)} />
      ) : (
        <Row label={t("account.name")} action={<Button variant="ghost" onClick={() => setEditName(true)}>{t("account.edit")}</Button>}>
          {me.name}
        </Row>
      )}
      <Row label={t("account.email")}>{me.email}</Row>
      <Row label={t("account.phone")} action={<Button variant="ghost" onClick={() => setEditPhone(true)}>{me.phoneVerified ? t("account.change") : t("account.verify")}</Button>}>
        {me.phoneMasked ?? t("account.noPhone")}{" "}
        <Badge tone={me.phoneVerified ? "success" : "warning"}>{me.phoneVerified ? t("account.verified") : t("account.notVerified")}</Badge>
      </Row>
      <Modal open={editPhone} onClose={() => setEditPhone(false)} title={t("account.phoneTitle")}>
        <PhoneVerify onVerified={() => { setEditPhone(false); toast.show(t("account.saved")); }} />
      </Modal>
    </Card>
  );
}
