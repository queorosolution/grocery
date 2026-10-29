"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { Button, Card, ConfirmDialog } from "@/components/ui";
import { useLogout } from "@/features/auth/api";
import { t } from "@/lib/i18n";

/** Sign out here or everywhere. */
export function SignOutCard() {
  const logout = useLogout();
  const router = useRouter();
  const [all, setAll] = useState(false);
  const leave = (everywhere: boolean) => logout.mutate(everywhere, { onSettled: () => router.replace("/") });
  return (
    <Card className="flex flex-col gap-3 p-4">
      <Button variant="secondary" loading={logout.isPending && !all} onClick={() => leave(false)}>{t("account.signOut")}</Button>
      <Button variant="ghost" onClick={() => setAll(true)}>{t("account.signOutAll")}</Button>
      <ConfirmDialog open={all} title={t("account.signOutAll")} message={t("account.signOutAllBody")} confirmLabel={t("account.signOutAll")}
        loading={logout.isPending} onConfirm={() => leave(true)} onClose={() => setAll(false)} />
    </Card>
  );
}
