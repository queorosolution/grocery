import { describe, expect, it } from "vitest";
import { MAX_LINE, MAX_RULES, buildHeadersFile, checkSite } from "./site.mjs";

const BASE = "/*\n  X-Content-Type-Options: nosniff\n";
const hosts = { apiBase: "https://api.example.test/v1" };
const page = (script) => `<html><body><script>${script}</script></body></html>`;

const redirects = [{ from: "/order/:id", to: "/order/_/" }];

const pages = {
  "index.html": page("self.__next_f=[]"),
  "orders/index.html": page("self.__next_f.push([1,'orders'])"),
  "order/_/index.html": page("self.__next_f.push([1,'order'])"),
  "s/_/index.html": page("self.__next_f.push([1,'shop-placeholder'])"),
  "404.html": page("self.__next_f.push([1,'404'])"),
};

describe("buildHeadersFile + checkSite", () => {
  const text = buildHeadersFile(BASE, pages, hosts, redirects);

  it("keeps the base headers and adds a policy per page", () => {
    expect(text).toContain("X-Content-Type-Options: nosniff");
    expect(text).toContain("\n/orders/\n");
    expect(text).toContain("\n/order/:id\n");
    expect(text).toContain("\n/order/:id/\n");
    expect(text).toContain("\n/s/_/\n");
  });

  it("passes its own check, including rewritten id pages and pages served as 404", () => {
    expect(checkSite(text, pages, redirects)).toEqual([]);
  });

  it("fails when a page has an inline script the policy does not list", () => {
    const changed = { ...pages, "orders/index.html": page("different()") };
    expect(checkSite(text, changed, redirects).join("\n")).toMatch(/orders\/index\.html: inline script/);
  });

  it("fails on unsafe-inline", () => {
    const bad = text.replace("script-src 'self'", "script-src 'self' 'unsafe-inline'");
    expect(checkSite(bad, pages, redirects).join("\n")).toMatch(/unsafe/);
  });

  it("fails on an inline event handler", () => {
    const withHandler = { ...pages, "index.html": '<button onclick="x()">a</button>' + pages["index.html"] };
    expect(checkSite(text, withHandler, redirects).join("\n")).toMatch(/event handler/);
  });

  it("fails when a page would get two policies (a missing detach)", () => {
    const merged = text.replaceAll("  ! Content-Security-Policy\n", "");
    expect(checkSite(merged, pages, redirects).length).toBeGreaterThan(0);
  });

  it("fails over Cloudflare's limits", () => {
    const many = Object.fromEntries(Array.from({ length: MAX_RULES + 1 }, (_, i) => [`p${i}/index.html`, page(`s${i}()`)]));
    expect(checkSite(buildHeadersFile(BASE, many, hosts), many).join("\n")).toMatch(/header rules/);
    const heavy = { "index.html": page("a()".repeat(10) + "x"), "big/index.html": Array.from({ length: 80 }, (_, i) => page(`n${i}()`)).join("") };
    expect(checkSite(buildHeadersFile(BASE, heavy, hosts), heavy).join("\n")).toMatch(new RegExp(`allows ${MAX_LINE}`));
  });
});
