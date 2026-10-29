import { describe, expect, it } from "vitest";
import { isAllowedPayHereAction, safeNext, safeUrl } from "./safeUrl";

describe("safeNext (XSS-03)", () => {
  it.each([
    ["//evil.com", "/"],
    ["/\\evil.com", "/"],
    ["\\\\evil.com", "/"],
    ["javascript:alert(1)", "/"],
    ["https://evil.com", "/"],
    ["http://evil.com/x", "/"],
    ["evil.com", "/"],
    ["", "/"],
    ["/ok\n//evil.com", "/"],
    ["/\t/evil.com", "/"],
    ["/%5Cevil.com/../..", "/%5Cevil.com/../.."], // percent-encoded stays a path on our own site
  ])("%j -> %j", (input, want) => {
    expect(safeNext(input)).toBe(want);
  });

  it("keeps real paths with query and hash", () => {
    expect(safeNext("/orders")).toBe("/orders");
    expect(safeNext("/s/krishna-super/?cat=rice#top")).toBe("/s/krishna-super/?cat=rice#top");
  });

  it("falls back for null and undefined", () => {
    expect(safeNext(null)).toBe("/");
    expect(safeNext(undefined)).toBe("/");
  });
});

describe("safeUrl (XSS-04)", () => {
  it.each([
    "javascript:alert(1)",
    "JaVaScRiPt:alert(1)",
    "java\nscript:alert(1)",
    "data:text/html,<script>alert(1)</script>",
    "http://img.example.test/a.png",
    "//img.example.test/a.png",
    "https://user:pass@img.example.test/a.png",
    " https://img.example.test/a.png",
    "",
    "not a url",
  ])("refuses %j", (input) => {
    expect(safeUrl(input)).toBeNull();
  });

  it("accepts plain https", () => {
    expect(safeUrl("https://img.example.test/a.png")).toBe("https://img.example.test/a.png");
  });

  it("can limit the host", () => {
    expect(safeUrl("https://img.example.test/a.png", ["img.example.test"])).not.toBeNull();
    expect(safeUrl("https://evil.test/a.png", ["img.example.test"])).toBeNull();
    expect(safeUrl("https://img.example.test.evil.test/a.png", ["img.example.test"])).toBeNull();
  });

  it("returns null for null and undefined", () => {
    expect(safeUrl(null)).toBeNull();
    expect(safeUrl(undefined)).toBeNull();
  });
});

describe("isAllowedPayHereAction (XSS-05)", () => {
  it("allows exactly the two checkout URLs", () => {
    expect(isAllowedPayHereAction("https://sandbox.payhere.lk/pay/checkout")).toBe(true);
    expect(isAllowedPayHereAction("https://www.payhere.lk/pay/checkout")).toBe(true);
  });

  it.each([
    "https://evil.com/pay",
    "https://sandbox.payhere.lk/pay/checkout/",
    "https://sandbox.payhere.lk/pay/checkout?x=1",
    "http://sandbox.payhere.lk/pay/checkout",
    "https://sandbox.payhere.lk.evil.com/pay/checkout",
    "https://www.payhere.lk@evil.com/pay/checkout",
    "//www.payhere.lk/pay/checkout",
    "javascript:alert(1)",
    "",
  ])("refuses %j", (action) => {
    expect(isAllowedPayHereAction(action)).toBe(false);
  });
});
