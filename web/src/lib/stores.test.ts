import { beforeEach, describe, expect, it } from "vitest";
import { readCartRef, writeCartRef } from "./cartStore";
import { readPin, writePin } from "./pinStore";

beforeEach(() => window.localStorage.clear());

describe("cartStore", () => {
  const ref = { shopId: "s1", shopSlug: "k", shopName: "K", tenantId: "t", publicKey: "pk", cartId: "c1" };
  it("round-trips, and gives the same object while nothing changed", () => {
    writeCartRef(ref);
    expect(readCartRef()).toEqual(ref);
    expect(readCartRef()).toBe(readCartRef());
    writeCartRef(null);
    expect(readCartRef()).toBeNull();
  });
  it("ignores text that is not a cart", () => {
    for (const raw of ["{", "[]", '{"cartId":"c"}', JSON.stringify({ ...ref, cartId: 5 })]) {
      window.localStorage.setItem("gp_cart", raw);
      expect(readCartRef()).toBeNull();
    }
  });
});

describe("pinStore", () => {
  it("keeps two decimals only", () => {
    expect(writePin({ lat: 6.93517, lng: 79.84821 })).toBe(true);
    expect(readPin()).toEqual({ lat: 6.94, lng: 79.85 });
  });
  it("refuses a place outside Sri Lanka and ignores bad stored text", () => {
    expect(writePin({ lat: 51.5, lng: -0.12 })).toBe(false);
    window.localStorage.setItem("gp_pin", '{"lat":"x","lng":1}');
    expect(readPin()).toBeNull();
  });
});
