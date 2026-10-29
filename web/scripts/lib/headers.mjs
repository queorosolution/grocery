// Reads and applies a Cloudflare Pages `_headers` file, as documented at
// developers.cloudflare.com/pages/configuration/headers: rules are merged, the same
// header from two rules is joined with a comma, and "! Name" removes an inherited header.

/** @returns {{ path: string, lines: { detach: boolean, name: string, value: string }[] }[]} */
export function parseHeaders(text) {
  const rules = [];
  for (const raw of text.split("\n")) {
    if (raw.trim() === "" || raw.trim().startsWith("#")) continue;
    if (!/^\s/.test(raw)) {
      rules.push({ path: raw.trim(), lines: [] });
      continue;
    }
    const rule = rules.at(-1);
    if (!rule) throw new Error(`_headers: header line before any path: ${raw.trim()}`);
    const line = raw.trim();
    if (line.startsWith("! ")) {
      rule.lines.push({ detach: true, name: line.slice(2).trim().toLowerCase(), value: "" });
      continue;
    }
    const colon = line.indexOf(":");
    if (colon < 1) throw new Error(`_headers: not a header line: ${line}`);
    rule.lines.push({ detach: false, name: line.slice(0, colon).trim().toLowerCase(), value: line.slice(colon + 1).trim() });
  }
  return rules;
}

/** Does a `_headers` path pattern (":name" for one segment, "*" for the rest) match a URL path? */
export function matchesPattern(pattern, urlPath) {
  const source = pattern
    .split("*")
    .map((part) => part.split("/").map((seg) => (/^:[A-Za-z]\w*$/.test(seg) ? "[^/]+" : seg.replace(/[.+?^${}()|[\]\\]/g, "\\$&"))).join("/"))
    .join(".*");
  return new RegExp(`^${source}$`).test(urlPath);
}

/** The headers a request for `urlPath` would get. Keys are lower-case. */
export function headersFor(rules, urlPath) {
  const out = new Map();
  for (const rule of rules) {
    if (!matchesPattern(rule.path, urlPath)) continue;
    for (const line of rule.lines) {
      if (line.detach) out.delete(line.name);
      else out.set(line.name, out.has(line.name) ? `${out.get(line.name)}, ${line.value}` : line.value);
    }
  }
  return out;
}
