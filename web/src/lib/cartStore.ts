// Which cart this browser has: one shop at a time, in localStorage key `gp_cart`
// (docs/08 section 6). It holds no personal data. The public key is public by design.

export interface CartRef {
  shopId: string;
  shopSlug: string;
  shopName: string;
  tenantId: string;
  publicKey: string;
  cartId: string;
}

const KEY = "gp_cart";
const listeners = new Set<() => void>();
let cache: { raw: string | null; ref: CartRef | null } = { raw: null, ref: null };

const text = (v: unknown): v is string => typeof v === "string" && v.length > 0 && v.length < 200;

function parse(raw: string | null): CartRef | null {
  if (!raw) return null;
  try {
    const x = JSON.parse(raw) as Partial<Record<keyof CartRef, unknown>>;
    return text(x.shopId) && text(x.shopSlug) && text(x.shopName) && text(x.tenantId) && text(x.publicKey) && text(x.cartId)
      ? { shopId: x.shopId, shopSlug: x.shopSlug, shopName: x.shopName, tenantId: x.tenantId, publicKey: x.publicKey, cartId: x.cartId }
      : null;
  } catch {
    return null;
  }
}

function read(): string | null {
  try {
    return window.localStorage.getItem(KEY);
  } catch {
    return null; // private mode or blocked storage: the cart then lives until the tab closes
  }
}

/** Same object while the stored text is the same, as useSyncExternalStore needs. */
export function readCartRef(): CartRef | null {
  const raw = read();
  if (raw !== cache.raw) cache = { raw, ref: parse(raw) };
  return cache.ref;
}

export function writeCartRef(ref: CartRef | null): void {
  try {
    if (ref) window.localStorage.setItem(KEY, JSON.stringify(ref));
    else window.localStorage.removeItem(KEY);
  } catch {
    // Storage is blocked; nothing to save to.
  }
  cache = { raw: ref ? JSON.stringify(ref) : null, ref };
  listeners.forEach((l) => l());
}

export function subscribeCartRef(listener: () => void): () => void {
  listeners.add(listener);
  window.addEventListener("storage", listener); // another tab changed the cart
  return () => {
    listeners.delete(listener);
    window.removeEventListener("storage", listener);
  };
}
