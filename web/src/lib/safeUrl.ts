// Guards for values that come from outside: URLs from the API, ?next= and payment forms.
// Each one answers "is this safe?" and falls back to a harmless value.

/** The URL back to where the user was. Only same-site paths like "/orders" pass. */
export function safeNext(next: string | null | undefined): string {
  if (!next || !next.startsWith("/") || next.startsWith("//")) return "/";
  // Browsers read "\" as "/", and control characters can hide a scheme.
  if (/[\\\u0000-\u001f\u007f]/.test(next)) return "/";
  try {
    const u = new URL(next, "http://local.invalid");
    if (u.origin !== "http://local.invalid") return "/";
  } catch {
    return "/";
  }
  return next;
}

/** Returns the URL only when it is plain https (optionally on one of `hosts`), else null. */
export function safeUrl(value: string | null | undefined, hosts?: readonly string[]): string | null {
  if (!value || /[\u0000-\u001f\u007f\s]/.test(value)) return null;
  try {
    const u = new URL(value);
    if (u.protocol !== "https:" || u.username || u.password) return null;
    if (hosts && !hosts.includes(u.hostname)) return null;
    return u.href;
  } catch {
    return null;
  }
}

const PAYHERE_ACTIONS: ReadonlySet<string> = new Set([
  "https://sandbox.payhere.lk/pay/checkout",
  "https://www.payhere.lk/pay/checkout",
]);

/** The payment form may only post to one of the two PayHere checkout URLs, exactly. */
export function isAllowedPayHereAction(action: string): boolean {
  return PAYHERE_ACTIONS.has(action);
}
