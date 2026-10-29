import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { buildRedirects, findIdRoutes, toRule } from "./redirects.mjs";

async function app(files) {
  const root = await mkdtemp(path.join(os.tmpdir(), "routes-"));
  for (const f of files) {
    await mkdir(path.join(root, path.dirname(f)), { recursive: true });
    await writeFile(path.join(root, f), "export default function Page() { return null }");
  }
  return root;
}

describe("findIdRoutes", () => {
  it("finds id routes, ignores groups, fixed pages and the public shop page", async () => {
    const root = await app([
      "(customer)/page.tsx",
      "(customer)/orders/page.tsx",
      "(customer)/order/[id]/page.tsx",
      "(customer)/s/[slug]/page.tsx",
      "console/shop/[shopId]/orders/page.tsx",
      "console/shop/[shopId]/order/[orderId]/page.tsx",
      "console/layout.tsx",
    ]);
    expect(await findIdRoutes(root)).toEqual([
      "/console/shop/[shopId]/order/[orderId]",
      "/console/shop/[shopId]/orders",
      "/order/[id]",
    ]);
  });
});

describe("toRule", () => {
  it("turns folders into Cloudflare placeholders and the one placeholder file", () => {
    expect(toRule("/console/shop/[shopId]/order/[orderId]")).toEqual({
      from: "/console/shop/:shopId/order/:orderId",
      to: "/console/shop/_/order/_/",
    });
  });
});

describe("buildRedirects", () => {
  it("writes a 200 rewrite with and without the trailing slash", () => {
    const text = buildRedirects(["/order/[id]"]);
    expect(text).toContain("/order/:id  /order/_/  200");
    expect(text).toContain("/order/:id/  /order/_/  200");
    expect(text).toMatch(/^# GENERATED/);
  });

  it("has no catch-all that would hide pre-built pages", () => {
    expect(buildRedirects(["/order/[id]"])).not.toMatch(/\/s\/|\*/);
  });

  it("fails over the Cloudflare limit", () => {
    const routes = Array.from({ length: 51 }, (_, i) => `/r${i}/[id]`);
    expect(() => buildRedirects(routes)).toThrow(/100/);
  });
});
