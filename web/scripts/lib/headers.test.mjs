import { describe, expect, it } from "vitest";
import { headersFor, matchesPattern, parseHeaders } from "./headers.mjs";

describe("matchesPattern", () => {
  it("matches placeholders for one segment and splats for the rest", () => {
    expect(matchesPattern("/order/:id", "/order/abc")).toBe(true);
    expect(matchesPattern("/order/:id", "/order/a/b")).toBe(false);
    expect(matchesPattern("/order/:id/", "/order/abc/")).toBe(true);
    expect(matchesPattern("/*", "/anything/at/all/")).toBe(true);
    expect(matchesPattern("/_next/static/*", "/_next/static/chunk.js")).toBe(true);
    expect(matchesPattern("/", "/")).toBe(true);
    expect(matchesPattern("/", "/x/")).toBe(false);
  });

  it("treats dots and other characters literally", () => {
    expect(matchesPattern("/a.b/", "/aXb/")).toBe(false);
  });
});

describe("headersFor", () => {
  const rules = parseHeaders(`
# comment
/*
  X-A: base
  Content-Security-Policy: fallback

/page/
  ! Content-Security-Policy
  Content-Security-Policy: own
  X-A: more
`);

  it("merges and joins values with a comma", () => {
    expect(headersFor(rules, "/page/").get("x-a")).toBe("base, more");
  });

  it("lets a detach line replace an inherited header", () => {
    expect(headersFor(rules, "/page/").get("content-security-policy")).toBe("own");
    expect(headersFor(rules, "/other/").get("content-security-policy")).toBe("fallback");
  });

  it("rejects a header line before any path", () => {
    expect(() => parseHeaders("  X-A: 1\n")).toThrow();
  });
});
