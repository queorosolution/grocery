import { inSriLanka, type LatLng } from "./geo";

// The rough place the shopper chose for "shops near you". Two decimals is about 1 km: the
// server rounds the same way (docs/06 section 3), so nothing more exact is ever kept.

const KEY = "gp_pin";
const listeners = new Set<() => void>();
let cache: { raw: string | null; pin: LatLng | null } = { raw: null, pin: null };

const round2 = (n: number) => Math.round(n * 100) / 100;

function parse(raw: string | null): LatLng | null {
  if (!raw) return null;
  try {
    const x = JSON.parse(raw) as Partial<LatLng>;
    const pin = { lat: Number(x.lat), lng: Number(x.lng) };
    return Number.isFinite(pin.lat) && Number.isFinite(pin.lng) && inSriLanka(pin) ? pin : null;
  } catch {
    return null;
  }
}

function read(): string | null {
  try {
    return window.localStorage.getItem(KEY);
  } catch {
    return null;
  }
}

export function readPin(): LatLng | null {
  const raw = read();
  if (raw !== cache.raw) cache = { raw, pin: parse(raw) };
  return cache.pin;
}

/** Keeps the place rounded. A place outside Sri Lanka is refused (false). */
export function writePin(pin: LatLng | null): boolean {
  const rounded = pin && { lat: round2(pin.lat), lng: round2(pin.lng) };
  if (rounded && !inSriLanka(rounded)) return false;
  const raw = rounded ? JSON.stringify(rounded) : null;
  try {
    if (raw) window.localStorage.setItem(KEY, raw);
    else window.localStorage.removeItem(KEY);
  } catch {
    // Blocked storage: the choice lasts until the page closes.
  }
  cache = { raw, pin: rounded };
  listeners.forEach((l) => l());
  return true;
}

export function subscribePin(listener: () => void): () => void {
  listeners.add(listener);
  window.addEventListener("storage", listener);
  return () => {
    listeners.delete(listener);
    window.removeEventListener("storage", listener);
  };
}
