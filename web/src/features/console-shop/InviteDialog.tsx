"use client";

import { useState, type FormEvent } from "react";
import { Banner, Button, Checkbox, Field, Input, Modal, Select } from "@/components/ui";
import { errorText, fieldErrors } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import { useInvite, type InviteBody } from "./teamApi";

/** The link that works once. There is no e-mail sender, so the owner passes it on (docs/06 section 8). */
function LinkStep({ email, token, onClose }: { email: string; token: string; onClose: () => void }) {
  const [copied, setCopied] = useState(false);
  const link = `${window.location.origin}/?invite=${token}`;
  const copy = () => navigator.clipboard?.writeText(link).then(() => setCopied(true), () => undefined);
  return (
    <Modal open onClose={onClose} title={t("shopConsole.inviteLinkTitle")} footer={<Button className="flex-1" onClick={onClose}>{t("common.done")}</Button>}>
      <p className="mb-3">{t("shopConsole.inviteLinkBody", { email })}</p>
      <Input readOnly aria-label={t("shopConsole.inviteLinkLabel")} value={link} onFocus={(e) => e.currentTarget.select()} />
      <Button variant="secondary" className="mt-2" onClick={copy}>{copied ? t("shopConsole.copied") : t("shopConsole.copy")}</Button>
    </Modal>
  );
}

export function InviteDialog({ shopId, open, onClose }: { shopId: string; open: boolean; onClose: () => void }) {
  const invite = useInvite(shopId);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<InviteBody["role"]>("SHOP_STAFF");
  const [canEdit, setCanEdit] = useState(false);
  const [done, setDone] = useState<{ email: string; token: string } | null>(null);

  const close = () => { invite.reset(); setDone(null); setEmail(""); onClose(); };
  const submit = (e: FormEvent) => {
    e.preventDefault();
    const clean = email.trim();
    invite.mutate({ email: clean, role, canEditProducts: role === "SHOP_STAFF" && canEdit }, { onSuccess: (r) => setDone({ email: clean, token: r.token }) });
  };
  if (!open) return null;
  if (done) return <LinkStep email={done.email} token={done.token} onClose={close} />;
  const emailError = fieldErrors(invite.error, { email: t("auth.emailInvalid") }).email;
  return (
    <Modal open onClose={close} title={t("shopConsole.inviteTitle")}
      footer={<Button type="submit" form="invite-form" className="flex-1" loading={invite.isPending} disabled={!email.includes("@")}>{t("shopConsole.inviteSend")}</Button>}>
      <form id="invite-form" onSubmit={submit} className="flex flex-col gap-3">
        <Field label={t("shopConsole.inviteEmail")} error={emailError}>
          {(p) => <Input {...p} type="email" autoComplete="off" value={email} onChange={(e) => setEmail(e.target.value)} />}
        </Field>
        <Field label={t("shopConsole.inviteRole")}>
          {(p) => (
            <Select {...p} value={role} onChange={(e) => setRole(e.target.value as InviteBody["role"])}>
              <option value="SHOP_STAFF">{t("shopConsole.roleStaff")}</option>
              <option value="SHOP_DRIVER">{t("shopConsole.roleDriver")}</option>
            </Select>
          )}
        </Field>
        {role === "SHOP_STAFF" && <Checkbox label={t("shopConsole.canEdit")} checked={canEdit} onChange={setCanEdit} />}
        {invite.error && !emailError && <Banner tone="danger">{errorText(invite.error)}</Banner>}
      </form>
    </Modal>
  );
}
