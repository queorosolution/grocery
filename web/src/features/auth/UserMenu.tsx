"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui";
import { t } from "@/lib/i18n";
import { useLogout, useMe } from "./api";
import { useAuth } from "./AuthProvider";

const item = "block w-full px-4 py-3 text-left hover:bg-surface";

/** "Sign in" when signed out; otherwise a round initial that opens the account menu. */
export function UserMenu() {
  const { me, loading } = useMe();
  const { openSignIn } = useAuth();
  const logout = useLogout();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const away = (e: PointerEvent) => {
      if (!box.current?.contains(e.target as Node)) setOpen(false);
    };
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("pointerdown", away);
    document.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("pointerdown", away);
      document.removeEventListener("keydown", esc);
    };
  }, [open]);

  if (loading) return <span aria-hidden="true" className="size-10" />; // keeps the bar from jumping
  if (!me) return <Button variant="secondary" onClick={openSignIn}>{t("auth.signIn")}</Button>;

  const signOut = () => logout.mutate(false, { onSettled: () => { setOpen(false); router.replace("/"); } });
  return (
    <div ref={box} className="relative">
      <button type="button" aria-haspopup="menu" aria-expanded={open} aria-label={t("menu.label")} onClick={() => setOpen(!open)}
        className="flex size-10 items-center justify-center rounded-pill bg-ink text-base font-semibold text-bg">
        {me.name.trim().charAt(0).toUpperCase() || "?"}
      </button>
      {open && (
        <div role="menu" className="absolute right-0 z-20 mt-2 w-60 overflow-hidden rounded-card border border-line bg-bg py-1 shadow-raise">
          <div className="border-b border-line px-4 py-3">
            <p className="truncate font-semibold">{me.name}</p>
            <p className="truncate text-sm text-muted">{me.email}</p>
          </div>
          <Link role="menuitem" href="/orders" className={item} onClick={() => setOpen(false)}>{t("menu.orders")}</Link>
          <Link role="menuitem" href="/account" className={item} onClick={() => setOpen(false)}>{t("menu.account")}</Link>
          {me.roles.length > 0 && <Link role="menuitem" href="/console" className={item} onClick={() => setOpen(false)}>{t("menu.console")}</Link>}
          <button type="button" role="menuitem" className={item} onClick={signOut}>{t("menu.signOut")}</button>
        </div>
      )}
    </div>
  );
}
