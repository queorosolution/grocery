"use client";

import { usePathname } from "next/navigation";
import { useSyncExternalStore } from "react";

// After a Cloudflare rewrite the page is the placeholder file, so `params` says "_".
// The real ids are in the URL, and this is the only code that reads them (docs/08 section 11.2).

const PLACEHOLDER = "_";
const ID = /^[A-Za-z0-9_-]{1,64}$/;

/** matchRoute("/order/:id", "/order/abc/") = { id: "abc" }. Null when it does not match. */
export function matchRoute(pattern: string, pathname: string): Record<string, string> | null {
  const want = pattern.split("/").filter(Boolean);
  const got = pathname.split("/").filter(Boolean);
  if (want.length !== got.length) return null;
  const out: Record<string, string> = {};
  for (const [i, part] of want.entries()) {
    const seg = got[i] ?? "";
    if (!part.startsWith(":")) {
      if (part !== seg) return null;
      continue;
    }
    let value: string;
    try {
      value = decodeURIComponent(seg);
    } catch {
      return null;
    }
    if (value === PLACEHOLDER || !ID.test(value)) return null;
    out[part.slice(1)] = value;
  }
  return out;
}

const noSubscription = () => () => {};

/**
 * The browser's path, or null until the page has mounted. The HTML on disk was built for the
 * placeholder URL, so the first render must not read the real one: React would find the server
 * HTML and the browser disagree (hydration error). One extra render later it is correct.
 */
export function useClientPathname(): string | null {
  const pathname = usePathname();
  const mounted = useSyncExternalStore(noSubscription, () => true, () => false);
  return mounted ? (pathname ?? "") : null;
}

/** All ids of the current URL for a pattern, or null (placeholder build, other page, not mounted yet). */
export function useRouteParams(pattern: string): Record<string, string> | null {
  const pathname = useClientPathname();
  return pathname === null ? null : matchRoute(pattern, pathname);
}

/** One id of the current URL: useRouteId("/order/:id") reads "id". */
export function useRouteId(pattern: string, name = "id"): string | null {
  return useRouteParams(pattern)?.[name] ?? null;
}
