import { describe, expect, it } from "vitest";
import { cleanShopList } from "./shops.mjs";

describe("cleanShopList", () => {
  it("keeps good shops, sorted, and only https logos", () => {
    const out = cleanShopList({
      data: [
        { slug: "zeta", name: "Zeta", logoUrl: "https://img.example.test/z.png" },
        { slug: "alpha", name: "Alpha", logoUrl: "javascript:alert(1)" },
      ],
    });
    expect(out).toEqual([
      { slug: "alpha", name: "Alpha", logoUrl: null },
      { slug: "zeta", name: "Zeta", logoUrl: "https://img.example.test/z.png" },
    ]);
  });

  it("drops anything that would be a bad file path or title", () => {
    const out = cleanShopList({
      data: [
        { slug: "../etc", name: "x" },
        { slug: "Has Space", name: "x" },
        { slug: "ok", name: "" },
        { slug: "ok2", name: "y".repeat(121) },
        { slug: 5, name: "x" },
        null,
      ],
    });
    expect(out).toEqual([]);
  });

  it("returns an empty list for an unexpected answer", () => {
    expect(cleanShopList(undefined)).toEqual([]);
    expect(cleanShopList({ data: "nope" })).toEqual([]);
  });
});
