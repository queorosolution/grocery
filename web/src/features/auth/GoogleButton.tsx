"use client";

import clsx from "clsx";
import { useEffect, useRef, useState } from "react";
import { Banner, Button } from "@/components/ui";
import { env } from "@/lib/env";
import { t } from "@/lib/i18n";
import { renderGoogleButton } from "./google";

interface GoogleButtonProps {
  onToken: (idToken: string) => void;
  /** While the token is being checked: the button stays but cannot be pressed twice. */
  busy?: boolean;
}

/** Google's own "Continue with Google" button. It is drawn by Google's script, inside our box. */
export function GoogleButton({ onToken, busy }: GoogleButtonProps) {
  const box = useRef<HTMLDivElement>(null);
  const latest = useRef(onToken);
  const [attempt, setAttempt] = useState(0);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    latest.current = onToken;
  });

  useEffect(() => {
    if (!env.googleClientId || !box.current) return;
    setFailed(false);
    renderGoogleButton(box.current, (token) => latest.current(token)).catch(() => setFailed(true));
  }, [attempt]);

  if (!env.googleClientId) return <Banner>{t("auth.notConfigured")}</Banner>;
  if (failed) {
    return (
      <div className="flex flex-col items-start gap-2">
        <Banner tone="danger" className="w-full">{t("auth.googleFailed")}</Banner>
        <Button variant="secondary" onClick={() => setAttempt((n) => n + 1)}>{t("common.retry")}</Button>
      </div>
    );
  }
  return <div ref={box} aria-busy={busy || undefined} className={clsx("flex min-h-10 justify-center", busy && "pointer-events-none opacity-50")} />;
}
