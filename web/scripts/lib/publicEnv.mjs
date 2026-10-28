import { readdir, readFile } from "node:fs/promises";
import path from "node:path";

// The only NEXT_PUBLIC_ names allowed (docs/13 section 3). Everything with this prefix
// ships to every browser, so a new name needs a deliberate edit here and in the doc.
export const ALLOWED = new Set([
  "NEXT_PUBLIC_API_BASE",
  "NEXT_PUBLIC_CART_API_BASE",
  "NEXT_PUBLIC_GOOGLE_CLIENT_ID",
  "NEXT_PUBLIC_MAPS_KEY",
  "NEXT_PUBLIC_TURNSTILE_SITE_KEY",
  "NEXT_PUBLIC_IMG_BASE",
  "NEXT_PUBLIC_PLATFORM_NAME",
  "NEXT_PUBLIC_PLATFORM_DOMAIN",
  "NEXT_PUBLIC_ACCENT",
]);

const SKIP = new Set(["node_modules", ".next", "out", ".generated", ".git"]);
// This file lists the names, and the check's own test uses a made-up one.
const SELF = /(publicEnv|check-public-env)(\.test)?\.mjs$/;

/** Names found in `text` that are not allowed. */
export function unknownNames(text) {
  return [...new Set(text.match(/NEXT_PUBLIC_[A-Z0-9_]+/g) ?? [])].filter((n) => !ALLOWED.has(n));
}

export async function scan(dir, problems = []) {
  for (const e of await readdir(dir, { withFileTypes: true })) {
    if (SKIP.has(e.name)) continue;
    const full = path.join(dir, e.name);
    if (e.isDirectory()) await scan(full, problems);
    else if (!SELF.test(full) && /\.(m?[jt]sx?|json|css|html|md|ya?ml|env.*|example)$|^\.env/.test(e.name)) {
      for (const name of unknownNames(await readFile(full, "utf8"))) problems.push(`${full}: ${name}`);
    }
  }
  return problems;
}
