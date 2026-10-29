"use client";

import Link from "next/link";
import { useEffect, type ReactNode } from "react";
import { Button, EmptyState, Spinner } from "@/components/ui";
import type { Role } from "@/lib/enums.gen";
import { t } from "@/lib/i18n";
import { useMe } from "./api";
import { useAuth } from "./AuthProvider";
import { consoleRoles, hasRole } from "./roles";
import { TotpCodeScreen, TotpSetupScreen } from "./TotpScreens";

// These guards only decide what to show. The API checks every call again, so a person who
// skips a guard still gets 401, 403 or 404 from the server (docs/08 section 9).

const Centered = ({ children }: { children: ReactNode }) => <div className="flex min-h-[50dvh] items-center justify-center">{children}</div>;

export function RequireUser({ children }: { children: ReactNode }) {
  const { me, loading, failed, retry } = useMe();
  const { openSignIn } = useAuth();
  const signedOut = !loading && !failed && !me;
  useEffect(() => {
    if (signedOut) openSignIn();
  }, [signedOut, openSignIn]);

  if (loading) return <Centered><Spinner /></Centered>;
  if (failed && !me) {
    return <EmptyState title={t("guard.failedTitle")} body={t("guard.failedBody")} action={<Button onClick={retry}>{t("common.retry")}</Button>} />;
  }
  if (!me) {
    return <EmptyState title={t("guard.signInTitle")} body={t("guard.signInBody")} action={<Button onClick={openSignIn}>{t("auth.signIn")}</Button>} />;
  }
  return <>{children}</>;
}

/** Must sit inside RequireUser. */
export function RequireRole({ roles, children }: { roles: readonly Role[]; children: ReactNode }) {
  const { me } = useMe();
  if (!hasRole(me, ...roles)) {
    return (
      <EmptyState title={t("guard.deniedTitle")} body={t("guard.deniedBody")}
        action={<Link href="/" className="font-semibold text-accent underline">{t("notFound.home")}</Link>} />
    );
  }
  return <>{children}</>;
}

/** Super admins also need a fresh authenticator code per sign-in (docs/06 section 4). */
export function RequireTotp({ children }: { children: ReactNode }) {
  const { me } = useMe();
  if (me?.adminTotp === "SETUP_NEEDED") return <TotpSetupScreen />;
  if (me?.adminTotp === "CODE_NEEDED") return <TotpCodeScreen />;
  return <>{children}</>;
}

/** Everything under /console needs a signed-in team member. */
export function ConsoleGate({ children }: { children: ReactNode }) {
  return (
    <RequireUser>
      <RequireRole roles={consoleRoles}>{children}</RequireRole>
    </RequireUser>
  );
}

/** Everything under /console/admin: super admin, and past the authenticator step. */
export function AdminGate({ children }: { children: ReactNode }) {
  return (
    <RequireRole roles={["SUPER_ADMIN"]}>
      <RequireTotp>{children}</RequireTotp>
    </RequireRole>
  );
}
