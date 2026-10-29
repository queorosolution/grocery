import { env } from "@/lib/env";
import { loadScript } from "@/lib/loadScript";

// Google Identity Services, loaded only when the sign-in modal opens. Only the calls we use are typed.

interface GoogleId {
  initialize(o: { client_id: string; callback: (r: { credential?: string }) => void; cancel_on_tap_outside?: boolean }): void;
  renderButton(el: HTMLElement, o: Record<string, string | number>): void;
}

const SRC = "https://accounts.google.com/gsi/client";

// GIS wants one initialize per page, so the latest listener is kept here.
let listener: ((idToken: string) => void) | null = null;
let initialized = false;

export async function renderGoogleButton(el: HTMLElement, onToken: (idToken: string) => void): Promise<void> {
  await loadScript(SRC);
  const id = (window as unknown as { google?: { accounts?: { id?: GoogleId } } }).google?.accounts?.id;
  if (!id) throw new Error("Google sign-in is not available");
  listener = onToken;
  if (!initialized) {
    id.initialize({ client_id: env.googleClientId, callback: (r) => r.credential && listener?.(r.credential), cancel_on_tap_outside: false });
    initialized = true;
  }
  id.renderButton(el, { type: "standard", theme: "outline", size: "large", text: "continue_with", shape: "pill", width: Math.min(320, el.clientWidth || 280) });
}
