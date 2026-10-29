"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { createContext, useCallback, useContext, useEffect, useMemo, useState, useSyncExternalStore, type ReactNode } from "react";
import { setAuthLostHandler } from "@/lib/api";
import { forgetUser } from "./api";
import { SignInModal } from "./SignInModal";
import type { Me } from "./types";

interface AuthContext {
  openSignIn: () => void;
}

const Context = createContext<AuthContext | null>(null);

/** Opens the sign-in modal from anywhere: `const { openSignIn } = useAuth()`. */
export function useAuth(): AuthContext {
  const value = useContext(Context);
  if (!value) throw new Error("useAuth needs <AuthProvider>");
  return value;
}

// An invite token is 32 random bytes in base64url (backend ids.Token).
const INVITE = /^[A-Za-z0-9_-]{43}$/;

// The invite link is `/?invite=<token>`. It is read from the address bar, and nothing on the
// server side has one, so the server snapshot is null.
const noSubscribe = () => () => {};
const readInvite = () => {
  const token = new URLSearchParams(window.location.search).get("invite");
  return token && INVITE.test(token) ? token : null;
};

/** Owns the sign-in modal, and what happens when a session is lost or an invite link is opened. */
export function AuthProvider({ children }: { children: ReactNode }) {
  const client = useQueryClient();
  const router = useRouter();
  const [asked, setAsked] = useState(false);
  const [dismissed, setDismissed] = useState(false);
  const [used, setUsed] = useState(false);
  const link = useSyncExternalStore(noSubscribe, readInvite, () => null);
  const invite = used ? null : link;
  // An invite link opens the modal by itself, once; closing it leaves the header button to reopen.
  const open = asked || (invite !== null && !dismissed);

  // A second 401 after the one refresh try (lib/api.ts): clear the person, ask them to sign in.
  useEffect(() => {
    setAuthLostHandler(() => {
      forgetUser(client);
      setAsked(true);
    });
    return () => setAuthLostHandler(null);
  }, [client]);

  const signedIn = useCallback(
    (me: Me) => {
      setAsked(false);
      const here = window.location.pathname + window.location.search;
      if (invite) {
        const clean = new URL(window.location.href);
        clean.searchParams.delete("invite");
        window.history.replaceState(null, "", clean);
        setUsed(true);
        if (me.roles.length > 0) router.push("/console");
        return;
      }
      // Team members do not need a delivery address or a phone to work.
      if (me.needsOnboarding && me.roles.length === 0 && !window.location.pathname.startsWith("/onboarding")) {
        router.push(`/onboarding?next=${encodeURIComponent(here)}`);
      }
    },
    [invite, router],
  );

  const value = useMemo(() => ({ openSignIn: () => setAsked(true) }), []);
  return (
    <Context.Provider value={value}>
      {children}
      <SignInModal open={open} invite={invite} onClose={() => { setAsked(false); setDismissed(true); }} onSignedIn={signedIn} />
    </Context.Provider>
  );
}
