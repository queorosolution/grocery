import { describe, expect, it } from "vitest";
import { matchRoute } from "./useRouteId";

describe("matchRoute", () => {
  it("reads ids from the real URL, with or without a trailing slash", () => {
    expect(matchRoute("/order/:id", "/order/abc123/")).toEqual({ id: "abc123" });
    expect(matchRoute("/order/:id", "/order/abc123")).toEqual({ id: "abc123" });
  });

  it("reads several ids", () => {
    expect(matchRoute("/console/shop/:shopId/order/:orderId", "/console/shop/s1/order/o9/")).toEqual({
      shopId: "s1",
      orderId: "o9",
    });
  });

  it("returns null on the placeholder page, where the id is '_'", () => {
    expect(matchRoute("/order/:id", "/order/_/")).toBeNull();
    expect(matchRoute("/console/shop/:shopId/order/:orderId", "/console/shop/_/order/_/")).toBeNull();
  });

  it("returns null for another page or a different depth", () => {
    expect(matchRoute("/order/:id", "/orders/")).toBeNull();
    expect(matchRoute("/order/:id", "/order/")).toBeNull();
    expect(matchRoute("/order/:id", "/order/a/b/")).toBeNull();
    expect(matchRoute("/s/:slug", "/order/abc/")).toBeNull();
  });

  it("refuses ids that could be anything but an id", () => {
    expect(matchRoute("/order/:id", "/order/%3Cscript%3E/")).toBeNull();
    expect(matchRoute("/order/:id", "/order/%E0%A4%A/")).toBeNull(); // broken escape
    expect(matchRoute("/order/:id", `/order/${"a".repeat(65)}/`)).toBeNull();
  });
});
