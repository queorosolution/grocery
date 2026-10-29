import { createHash } from "node:crypto";

// Content-Security-Policy for a static export (docs/08 section 11.4, docs/10 section 5).
// Next.js puts small inline <script> tags in every page. We cannot use per-request
// nonces without a server, so each page's policy lists the SHA-256 of its own scripts.

/** Inline JavaScript in an HTML page: <script> without src, and not a data block. */
export function inlineScripts(html) {
  const found = [];
  for (const m of html.matchAll(/<script\b([^>]*)>([\s\S]*?)<\/script>/gi)) {
    const attrs = m[1] ?? "";
    const body = m[2] ?? "";
    if (/\bsrc\s*=/i.test(attrs) || body.trim() === "") continue;
    const type = /\btype\s*=\s*["']?([^"'\s>]+)/i.exec(attrs)?.[1]?.toLowerCase();
    if (type && !["module", "text/javascript", "application/javascript"].includes(type)) continue; // JSON data is not executed
    found.push(body);
  }
  return found;
}

/** Event-handler attributes (onclick=...) cannot be allowed by a hash. */
export function hasInlineHandlers(html) {
  return /<[a-z][^>]*\son[a-z]+\s*=/i.test(html.replace(/<script\b[\s\S]*?<\/script>/gi, ""));
}

export const hashOf = (script) => `'sha256-${createHash("sha256").update(script, "utf8").digest("base64")}'`;

const origin = (url) => {
  try {
    return new URL(url).origin;
  } catch {
    return "";
  }
};

/** The policy of docs/10 section 5 with this build's own hosts and the page's script hashes. */
export function buildCsp({ hashes = [], apiBase = "", cartApiBase = "", imgBase = "" }) {
  const own = (url) => origin(url);
  const scripts = ["'self'", ...hashes, "https://accounts.google.com/gsi/client", "https://maps.googleapis.com", "https://maps.gstatic.com"];
  const connect = ["'self'", own(apiBase), own(cartApiBase), "https://maps.googleapis.com", "https://places.googleapis.com",
    "https://accounts.google.com/gsi/", "https://*.r2.cloudflarestorage.com", "https://challenges.cloudflare.com"];
  const images = ["'self'", "data:", own(imgBase), "https://maps.gstatic.com", "https://maps.googleapis.com", "https://*.ggpht.com", "https://lh3.googleusercontent.com"];
  const list = (items) => [...new Set(items.filter(Boolean))].join(" ");
  return [
    "default-src 'self'",
    `script-src ${list(scripts)}`,
    "style-src 'self' 'unsafe-inline' https://accounts.google.com/gsi/style https://fonts.googleapis.com",
    `img-src ${list(images)}`,
    "font-src 'self' https://fonts.gstatic.com",
    `connect-src ${list(connect)}`,
    "frame-src https://accounts.google.com/gsi/ https://challenges.cloudflare.com",
    "form-action 'self' https://sandbox.payhere.lk https://www.payhere.lk",
    "frame-ancestors 'none'",
    "base-uri 'self'",
    "object-src 'none'",
    "upgrade-insecure-requests",
  ].join("; ");
}

/** The rewrites of public/_redirects: [{ from: "/order/:id", to: "/order/_/" }, ...]. */
export function parseRedirects(text) {
  return text
    .split("\n")
    .map((line) => line.trim().split(/\s+/))
    .filter((cols) => cols.length === 3 && cols[0]?.startsWith("/") && cols[2] === "200" && !cols[0].endsWith("/"))
    .map(([from, to]) => ({ from, to }));
}

/**
 * Where a built file is served, as `_headers` patterns. Cloudflare rewrites /order/<any id>/ to
 * the placeholder page but matches headers on the real URL, so a placeholder page gets the
 * pattern of the rewrite that points at it (with and without the trailing slash). Every other
 * page, including a pre-built shop page, is matched by its own exact path.
 * @param {string} file path inside out/, for example "order/_/index.html"
 * @param {{from:string,to:string}[]} redirects
 * @returns {string[]}
 */
export function pagePatterns(file, redirects = []) {
  const dir = file.replace(/(^|\/)index\.html$/, "").replace(/\.html$/, "");
  const url = dir === "" ? "/" : `/${dir}/`;
  const rewrite = redirects.find((r) => r.to === url);
  return rewrite ? [rewrite.from, `${rewrite.from}/`] : [url];
}

/** A URL a visitor could really open for a page (placeholders filled in), for checks. */
export const sampleUrl = (file, redirects = []) => (pagePatterns(file, redirects).at(-1) ?? "/").replace(/:\w+/g, "sample1");
