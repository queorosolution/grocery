"use client";

import { useState, type FormEvent } from "react";
import { Button, Field, Input } from "@/components/ui";
import { useUpdateName } from "@/features/auth/api";
import type { Me } from "@/features/auth/types";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";

/** Google gives a name already; this is the chance to say what the shop should call them. */
export function NameStep({ me, onDone }: { me: Me; onDone: () => void }) {
  const update = useUpdateName();
  const [name, setName] = useState(me.name);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    const clean = name.trim();
    if (clean === me.name) return onDone(); // nothing changed, nothing to send
    update.mutate(clean, { onSuccess: onDone });
  };
  return (
    <form onSubmit={submit} className="flex flex-col gap-3">
      <Field label={t("onboarding.nameLabel")} hint={t("onboarding.nameHint")} error={update.error ? errorText(update.error) : undefined}>
        {(p) => <Input {...p} maxLength={80} autoComplete="name" value={name} onChange={(e) => setName(e.target.value)} />}
      </Field>
      <Button type="submit" loading={update.isPending} disabled={name.trim().length < 1}>{t("common.next")}</Button>
    </form>
  );
}
