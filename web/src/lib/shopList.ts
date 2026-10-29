import { readFile } from "node:fs/promises";
import path from "node:path";
import { safeUrl } from "./safeUrl";

// Build-time only: reads what scripts/shop-slugs.mjs wrote. Used by the shop page's
// generateStaticParams and metadata, which run on the build machine, never in a browser.

export interface ShopListItem {
  slug: string;
  name: string;
  logoUrl: string | null;
}

const FILE = path.join(process.cwd(), ".generated", "shop-slugs.json");

export async function readShopList(): Promise<ShopListItem[]> {
  try {
    const raw: unknown = JSON.parse(await readFile(FILE, "utf8"));
    if (!Array.isArray(raw)) return [];
    return raw.flatMap((x: Partial<ShopListItem>) =>
      typeof x.slug === "string" && typeof x.name === "string"
        ? [{ slug: x.slug, name: x.name, logoUrl: safeUrl(x.logoUrl) }]
        : [],
    );
  } catch {
    return []; // no file yet (dev, or the API was down at build time)
  }
}
