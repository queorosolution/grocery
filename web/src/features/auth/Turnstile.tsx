"use client";

import { useEffect, useRef } from "react";
import { env } from "@/lib/env";
import { loadScript } from "@/lib/loadScript";

interface TurnstileApi {
  render(el: HTMLElement, o: Record<string, unknown>): string;
  reset(id: string): void;
  remove(id: string): void;
}

const SRC = "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit";
const api = () => (window as unknown as { turnstile?: TurnstileApi }).turnstile;

interface TurnstileProps {
  /** A token when the check passes; null when it expires, fails or is reset. */
  onToken: (token: string | null) => void;
  /** Change this number to ask for a new check (a token works once). */
  resetKey?: number;
}

/** Cloudflare's bot check. The token goes to our API, which verifies it with the secret key. */
export function Turnstile({ onToken, resetKey = 0 }: TurnstileProps) {
  const box = useRef<HTMLDivElement>(null);
  const widget = useRef<string | null>(null);
  const latest = useRef(onToken);

  useEffect(() => {
    latest.current = onToken;
  });

  useEffect(() => {
    let gone = false;
    loadScript(SRC).then(
      () => {
        const ts = api();
        if (gone || !ts || !box.current) return;
        widget.current = ts.render(box.current, {
          sitekey: env.turnstileSiteKey,
          callback: (token: string) => latest.current(token),
          "expired-callback": () => latest.current(null),
          "error-callback": () => latest.current(null),
        });
      },
      () => latest.current(null),
    );
    return () => {
      gone = true;
      if (widget.current) api()?.remove(widget.current);
      widget.current = null;
    };
  }, []);

  useEffect(() => {
    if (resetKey > 0 && widget.current) {
      api()?.reset(widget.current);
      latest.current(null);
    }
  }, [resetKey]);

  return <div ref={box} className="min-h-[65px]" />;
}
