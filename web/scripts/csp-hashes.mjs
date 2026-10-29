// Post-build: writes out/_headers with one Content-Security-Policy rule per page that
// lists the SHA-256 of that page's inline scripts (docs/08 section 11.4).
//   node scripts/csp-hashes.mjs           write out/_headers, then check it
//   node scripts/csp-hashes.mjs --check   only check an existing out/_headers
// The build fails if any inline script has no hash.
import { readdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { parseRedirects } from "./lib/csp.mjs";
import { loadEnvFiles } from "./lib/env.mjs";
import { buildHeadersFile, checkSite } from "./lib/site.mjs";

loadEnvFiles();
const out = path.resolve("out");

async function htmlFiles(dir, prefix = "") {
  const found = {};
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const rel = prefix ? `${prefix}/${entry.name}` : entry.name;
    if (entry.isDirectory()) Object.assign(found, await htmlFiles(path.join(dir, entry.name), rel));
    else if (entry.name.endsWith(".html")) found[rel] = await readFile(path.join(dir, entry.name), "utf8");
  }
  return found;
}

const pages = await htmlFiles(out);
const headersPath = path.join(out, "_headers");
const redirects = parseRedirects(await readFile(path.resolve("public/_redirects"), "utf8"));

if (!process.argv.includes("--check")) {
  const base = await readFile(path.resolve("public/_headers"), "utf8");
  const text = buildHeadersFile(base, pages, {
    apiBase: process.env.NEXT_PUBLIC_API_BASE,
    cartApiBase: process.env.NEXT_PUBLIC_CART_API_BASE,
    imgBase: process.env.NEXT_PUBLIC_IMG_BASE,
  }, redirects);
  await writeFile(headersPath, text);
}

const problems = checkSite(await readFile(headersPath, "utf8"), pages, redirects);
if (problems.length > 0) {
  console.error(`CSP check failed:\n${problems.map((p) => `  - ${p}`).join("\n")}`);
  process.exit(1);
}
console.log(`CSP ok: ${Object.keys(pages).length} pages`);
