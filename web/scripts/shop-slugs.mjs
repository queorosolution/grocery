// Before `next build`: asks the API which shops are active and saves them for the
// shop page's generateStaticParams (docs/08 section 11.1).
// If the API is down the list is empty and the build still succeeds: a new shop is
// shown by not-found.tsx until the next build.
import { mkdir, writeFile } from "node:fs/promises";
import { loadEnvFiles } from "./lib/env.mjs";
import { cleanShopList } from "./lib/shops.mjs";

loadEnvFiles();

async function fetchShops() {
  const base = process.env.NEXT_PUBLIC_API_BASE;
  if (!base) throw new Error("NEXT_PUBLIC_API_BASE is not set");
  const res = await fetch(`${base.replace(/\/+$/, "")}/shops?fields=slug`, {
    headers: { Accept: "application/json" },
    signal: AbortSignal.timeout(10_000),
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return cleanShopList(await res.json());
}

let shops = [];
try {
  shops = await fetchShops();
  console.log(`shop-slugs: ${shops.length} active shops`);
} catch (err) {
  console.warn(`shop-slugs: could not load shops (${err instanceof Error ? err.message : "error"}); building without shop pages`);
}

await mkdir(".generated", { recursive: true });
await writeFile(".generated/shop-slugs.json", `${JSON.stringify(shops, null, 2)}\n`);
