// The only place that reads NEXT_PUBLIC_* (docs/13 section 3). Next.js replaces
// each `process.env` read below at build time, so every name is written out.
// Everything here ships to every browser: never put a secret in it.

function pick(name: string, value: string | undefined, devDefault: string): string {
  const v = value?.trim();
  if (v) return v;
  if (process.env.NODE_ENV === "production") throw new Error(`Missing build variable ${name}`);
  return devDefault;
}

const trimSlash = (v: string) => v.replace(/\/+$/, "");

// The accent goes into a style attribute, so it must be a plain hex colour.
function hexColor(name: string, value: string): string {
  if (!/^#[0-9a-fA-F]{3,8}$/.test(value)) throw new Error(`${name} must be a hex colour like #0e7c66`);
  return value;
}

export const env = {
  apiBase: trimSlash(pick("NEXT_PUBLIC_API_BASE", process.env.NEXT_PUBLIC_API_BASE, "http://localhost:8080/v1")),
  cartApiBase: trimSlash(pick("NEXT_PUBLIC_CART_API_BASE", process.env.NEXT_PUBLIC_CART_API_BASE, "")),
  googleClientId: pick("NEXT_PUBLIC_GOOGLE_CLIENT_ID", process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID, ""),
  mapsKey: pick("NEXT_PUBLIC_MAPS_KEY", process.env.NEXT_PUBLIC_MAPS_KEY, ""),
  turnstileSiteKey: pick("NEXT_PUBLIC_TURNSTILE_SITE_KEY", process.env.NEXT_PUBLIC_TURNSTILE_SITE_KEY, ""),
  imgBase: trimSlash(pick("NEXT_PUBLIC_IMG_BASE", process.env.NEXT_PUBLIC_IMG_BASE, "")),
  platformName: pick("NEXT_PUBLIC_PLATFORM_NAME", process.env.NEXT_PUBLIC_PLATFORM_NAME, "NearShop"),
  platformDomain: pick("NEXT_PUBLIC_PLATFORM_DOMAIN", process.env.NEXT_PUBLIC_PLATFORM_DOMAIN, "localhost"),
  accent: hexColor("NEXT_PUBLIC_ACCENT", pick("NEXT_PUBLIC_ACCENT", process.env.NEXT_PUBLIC_ACCENT, "#0e7c66")),
} as const;
